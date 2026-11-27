package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/store"
)

// validateBulkScale checks the request shape once; per-app rules (autoscale,
// quota) are applied to each app.
func validateBulkScale(req bulkAppsRequest) error {
	hasCount := req.Replicas != nil
	hasBounds := req.MinReplicas != nil || req.MaxReplicas != nil
	switch {
	case !hasCount && !hasBounds:
		return errors.New("scale needs replicas, or min_replicas and max_replicas")
	case hasCount && *req.Replicas < 1:
		return errors.New("replicas must be at least 1")
	case hasBounds && (req.MinReplicas == nil || req.MaxReplicas == nil):
		return errors.New("min_replicas and max_replicas must be set together")
	case hasBounds && (*req.MinReplicas < 1 || *req.MaxReplicas < *req.MinReplicas):
		return errors.New("min_replicas must be at least 1 and max_replicas must not be below it")
	}
	return nil
}

// bulkScale sets replicas on a plain app, or min and max on an autoscaled one.
// An app of the other kind is skipped with a reason so a mixed selection never
// takes the replica count away from the autoscaler.
func (rt *Router) bulkScale(ctx context.Context, req bulkAppsRequest, svc store.DesiredService) (string, string) {
	c := rt.autoscaleController()
	var pol autoscale.Policy
	autoscaled := false
	if c != nil {
		p, err := c.Store.GetAutoscalePolicy(ctx, svc.Name)
		switch {
		case err == nil:
			pol, autoscaled = p, true
		case !errors.Is(err, autoscale.ErrNotFound):
			rt.logger.Error("api: bulk apps: read autoscale policy failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
			return bulkStatusError, "autoscale check failed"
		}
	}
	if autoscaled {
		return rt.bulkScaleAutoscaled(ctx, c, req, pol)
	}
	if req.Replicas == nil {
		return bulkStatusSkipped, "no autoscale policy: set replicas instead of min and max"
	}
	n := *req.Replicas
	if err := rt.checkReplicaCap(ctx, n); err != nil {
		return bulkStatusSkipped, err.Error()
	}
	if max(svc.Replicas, 1) == n {
		return bulkStatusSkipped, fmt.Sprintf("already at %d replicas", n)
	}
	next := svc
	next.Replicas = n
	release, msg := rt.bulkResizeQuota(ctx, svc, next)
	if release == nil {
		return bulkStatusSkipped, msg
	}
	defer release()
	rebase := func(fresh store.DesiredService) store.DesiredService {
		fresh.Replicas = n
		return fresh
	}
	if _, err := store.SaveRebased(ctx, next, rt.apps.GetDesiredService, rt.apps.SaveDesiredService, rebase); err != nil {
		rt.logger.Error("api: bulk apps: scale failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return bulkStatusError, "scale failed"
	}
	rt.nudgeReconciler()
	return bulkStatusOK, fmt.Sprintf("%d to %d replicas", max(svc.Replicas, 1), n)
}

func (rt *Router) bulkScaleAutoscaled(ctx context.Context, c *autoscale.Controller, req bulkAppsRequest, pol autoscale.Policy) (string, string) {
	if req.MinReplicas == nil {
		return bulkStatusSkipped, "autoscaled: set min and max instead of replicas"
	}
	pol.MinReplicas, pol.MaxReplicas = *req.MinReplicas, *req.MaxReplicas
	pol = pol.Normalize()
	if err := pol.Validate(c.Ceiling); err != nil {
		return bulkStatusSkipped, err.Error()
	}
	if err := rt.checkReplicaCap(ctx, pol.MaxReplicas); err != nil {
		return bulkStatusSkipped, err.Error()
	}
	now := time.Now()
	ev := autoscale.Event{
		App: pol.App, Kind: autoscale.EventConfigured, At: now,
		Detail: fmt.Sprintf("bulk scale set %d to %d replicas on %s target %g", pol.MinReplicas, pol.MaxReplicas, pol.Metric, pol.Target),
	}
	if err := c.Store.SaveAutoscalePolicy(ctx, pol, ev, now); err != nil {
		rt.logger.Error("api: bulk apps: save autoscale policy failed", slog.String("error", err.Error()), slog.String("name", pol.App))
		return bulkStatusError, "scale failed"
	}
	if err := c.Reconcile(ctx, pol, now); err != nil {
		rt.logger.Warn("autoscale: reconcile after bulk scale failed", slog.String("name", pol.App), slog.String("error", err.Error()))
	}
	return bulkStatusOK, fmt.Sprintf("min %d, max %d", pol.MinReplicas, pol.MaxReplicas)
}

// bulkResizeQuota runs the single-app resize quota gate off-wire: a nil release
// means refused, with the reason in msg.
func (rt *Router) bulkResizeQuota(ctx context.Context, existing, next store.DesiredService) (release func(), msg string) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/", http.NoBody)
	release, ok := rt.admitAppResizeQuota(rec, req, existing, next)
	if ok {
		return release, ""
	}
	return nil, responseErrorMessage(rec, "quota denied the change")
}

// responseErrorMessage reads the error text a handler wrote into rec.
func responseErrorMessage(rec *httptest.ResponseRecorder, fallback string) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &body) == nil && body.Error != "" {
		return body.Error
	}
	return fallback
}

// bulkRollback sends an app back to the release before its current one through
// the single-app rollback handler, so freeze, approval and restore rules match.
func (rt *Router) bulkRollback(ctx context.Context, r *http.Request, svc store.DesiredService) (string, string) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
	if err != nil {
		rt.logger.Error("api: bulk apps: list deploys failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return bulkStatusError, "rollback failed"
	}
	target := previousSucceededAttempt(attempts, svc.Image)
	if target == nil {
		return bulkStatusSkipped, "no previous release to roll back to"
	}
	sub := r.Clone(ctx)
	sub.Body, sub.ContentLength, sub.URL.RawQuery = http.NoBody, 0, ""
	sub.SetPathValue("name", svc.Name)
	sub.SetPathValue("deployId", target.ID)
	rec := httptest.NewRecorder()
	rt.handleRollbackToDeploy(rec, sub)
	switch {
	case rec.Code == http.StatusAccepted:
		var res deployTriggerResult
		if json.Unmarshal(rec.Body.Bytes(), &res) == nil && res.PendingApproval != nil {
			return bulkStatusSkipped, "protected environment: rollback waits for approval"
		}
		return bulkStatusOK, "rolled back to " + target.Image
	case rec.Code >= 400 && rec.Code < 500:
		return bulkStatusSkipped, responseErrorMessage(rec, "rollback refused")
	}
	return bulkStatusError, "rollback failed"
}

// previousSucceededAttempt returns the newest succeeded attempt whose image
// differs from the live one; attempts are newest first.
func previousSucceededAttempt(attempts []store.DeployAttempt, live string) *store.DeployAttempt {
	for i := range attempts {
		a := attempts[i]
		if a.Status == store.DeployAttemptStatusSucceeded && a.Image != live {
			return &a
		}
	}
	return nil
}
