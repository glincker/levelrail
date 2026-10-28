package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/canary"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	canaryDefaultWeight = 10
	canaryMaxWeight     = canary.MaxWeight
)

// CanaryStore is the store surface the canary routes need.
type CanaryStore interface {
	SetCanaryRelease(ctx context.Context, c store.CanaryRelease) error
	GetCanaryRelease(ctx context.Context, serviceName string) (store.CanaryRelease, bool, error)
	DeleteCanaryRelease(ctx context.Context, serviceName string) error
	ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error)
	GetCanaryPolicy(ctx context.Context, serviceName string) (store.CanaryPolicy, bool, error)
	SetCanaryOutcome(ctx context.Context, o store.CanaryOutcome) error
	GetCanaryOutcome(ctx context.Context, serviceName string) (store.CanaryOutcome, bool, error)
}

// CanaryProgress reports the live stage comparison of a staged canary.
type CanaryProgress interface {
	Progress(rel store.CanaryRelease) (canary.Progress, bool)
}

// SetCanaryProgress wires the canary reconciler's live view into GET responses.
func (rt *Router) SetCanaryProgress(p CanaryProgress) { rt.canaryProgress = p }

type canaryOutcomeResource struct {
	Outcome string    `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
	Image   string    `json:"image,omitempty"`
	At      time.Time `json:"at"`
}

type canaryResource struct {
	Active       bool                   `json:"active"`
	Image        string                 `json:"image,omitempty"`
	Weight       int                    `json:"weight"`
	CreatedAt    *time.Time             `json:"created_at,omitempty"`
	Staged       bool                   `json:"staged,omitempty"`
	Stages       []int                  `json:"stages,omitempty"`
	Stage        int                    `json:"stage,omitempty"`
	IntervalSecs int                    `json:"interval_secs,omitempty"`
	MaxErrorRate float64                `json:"max_error_rate,omitempty"`
	MaxP95Ratio  float64                `json:"max_p95_ratio,omitempty"`
	StatusReason string                 `json:"status_reason,omitempty"`
	Progress     *canary.Progress       `json:"progress,omitempty"`
	LastOutcome  *canaryOutcomeResource `json:"last_outcome,omitempty"`
}

type canaryRequest struct {
	Image        string  `json:"image"`
	Weight       *int    `json:"weight,omitempty"`
	Staged       *bool   `json:"staged,omitempty"`
	Stages       []int   `json:"stages,omitempty"`
	Interval     string  `json:"interval,omitempty"`
	MaxErrorRate float64 `json:"max_error_rate,omitempty"`
	MaxP95Ratio  float64 `json:"max_p95_ratio,omitempty"`
}

func (rt *Router) toCanaryResource(ctx context.Context, name string, c store.CanaryRelease, ok bool) canaryResource {
	if !ok {
		out := canaryResource{}
		if o, found, err := rt.canaries.GetCanaryOutcome(ctx, name); err == nil && found {
			out.LastOutcome = &canaryOutcomeResource{Outcome: o.Outcome, Reason: o.Reason, Image: o.Image, At: o.At}
		}
		return out
	}
	out := canaryResource{Active: true, Image: c.Image, Weight: c.Weight, CreatedAt: &c.CreatedAt, StatusReason: c.StatusReason}
	if c.Staged() {
		out.Staged, out.Stages, out.Stage = true, c.Stages, c.StageIndex+1
		out.IntervalSecs, out.MaxErrorRate, out.MaxP95Ratio = c.IntervalSecs, c.MaxErrorRate, c.MaxP95Ratio
		if rt.canaryProgress != nil {
			if p, seen := rt.canaryProgress.Progress(c); seen {
				out.Progress = &p
			}
		}
	}
	return out
}

func validCanaryWeight(w int) bool { return w >= 0 && w <= canaryMaxWeight }

func (rt *Router) handleGetCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get canary", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toCanaryResource(r.Context(), name, c, ok))
}

// resolveCanaryPolicy layers env defaults, the app's saved policy and the
// request, in that order, and reports whether the canary is staged.
func (rt *Router) resolveCanaryPolicy(ctx context.Context, name string, req canaryRequest) (canary.Policy, bool, error) {
	pol := canary.DefaultPolicy(canary.OSEnv)
	saved, hasSaved, err := rt.canaries.GetCanaryPolicy(ctx, name)
	if err != nil {
		return pol, false, fmt.Errorf("read canary policy: %w", err)
	}
	if hasSaved {
		pol.Stages = saved.Stages
		pol.Interval = time.Duration(saved.IntervalSecs) * time.Second
		pol.MaxErrorRate, pol.MaxP95Ratio = saved.MaxErrorRate, saved.MaxP95Ratio
	}
	staged := len(req.Stages) > 0 || (req.Staged != nil && *req.Staged) || (req.Staged == nil && req.Weight == nil && hasSaved)
	if !staged {
		return pol, false, nil
	}
	if len(req.Stages) > 0 {
		pol.Stages = req.Stages
	}
	if req.Interval != "" {
		d, err := time.ParseDuration(req.Interval)
		if err != nil || d < time.Second {
			return pol, true, errors.New("interval must be a duration of at least 1s, such as 5m")
		}
		pol.Interval = d
	}
	if req.MaxErrorRate != 0 {
		pol.MaxErrorRate = req.MaxErrorRate
	}
	if req.MaxP95Ratio != 0 {
		pol.MaxP95Ratio = req.MaxP95Ratio
	}
	return pol, true, pol.Validate()
}

// handleStartCanary clones the app into a hidden "<app>--canary" service
// running the new image. Ingress then sends weight percent of requests to it;
// a staged canary has that weight walked up by the canary reconciler.
func (rt *Router) handleStartCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req canaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Image = strings.TrimSpace(req.Image)
	weight := canaryDefaultWeight
	if req.Weight != nil {
		weight = *req.Weight
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}
	pol, staged, err := rt.resolveCanaryPolicy(r.Context(), name, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if staged && req.Weight != nil {
		writeError(w, http.StatusBadRequest, "weight cannot be combined with a staged canary, the first stage sets it")
		return
	}
	if staged {
		weight = pol.Weight(0)
	}
	if weight < 1 || weight > canaryMaxWeight {
		writeError(w, http.StatusBadRequest, "weight must be between 1 and 99")
		return
	}
	if store.IsCanaryService(name) {
		writeError(w, http.StatusBadRequest, "a canary cannot have its own canary")
		return
	}
	source, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: start canary: load app", err, slog.String("name", name))
		return
	}
	if reason := rt.canaryBlockReason(r.Context(), *source); reason != "" {
		writeError(w, http.StatusConflict, reason)
		return
	}
	if _, exists, err := rt.canaries.GetCanaryRelease(r.Context(), name); err != nil {
		rt.internalError(w, "api: start canary: check existing", err, slog.String("name", name))
		return
	} else if exists {
		writeError(w, http.StatusConflict, "a canary is already running for this app, promote or abort it first")
		return
	}

	if rt.imageResolver != nil {
		auth := rt.imageTrigger(r.Context(), *source, true, "canary start").Auth
		if _, err := rt.imageResolver.ResolveImage(r.Context(), req.Image, auth, true); err != nil {
			writeError(w, http.StatusBadRequest, "cannot resolve image "+req.Image+": "+err.Error())
			return
		}
	}

	canaryName := store.CanaryServiceName(name)
	clone := cloneDesiredService(*source, canaryName, nil)
	clone.Image = req.Image
	if err := rt.apps.SaveDesiredService(r.Context(), clone); err != nil {
		rt.internalError(w, "api: start canary: save clone", err, slog.String("name", name))
		return
	}
	if source.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), canaryName, source.ProjectID); err != nil {
			rt.rollbackClone(r.Context(), canaryName)
			rt.internalError(w, "api: start canary: assign project", err, slog.String("name", name))
			return
		}
	}
	if rt.secrets != nil {
		if err := rt.applyCloneExtras(r.Context(), *source, cloneAppRequest{NewName: canaryName, CopySecrets: true}); err != nil {
			rt.rollbackClone(r.Context(), canaryName)
			rt.internalError(w, "api: start canary: copy secrets", err, slog.String("name", name))
			return
		}
	}
	rel := store.CanaryRelease{ServiceName: name, CanaryService: canaryName, Image: req.Image, Weight: weight}
	if staged {
		rel.Stages, rel.IntervalSecs = pol.Stages, int(pol.Interval/time.Second)
		rel.MaxErrorRate, rel.MaxP95Ratio = pol.MaxErrorRate, pol.MaxP95Ratio
	}
	if err := rt.canaries.SetCanaryRelease(r.Context(), rel); err != nil {
		rt.rollbackClone(r.Context(), canaryName)
		rt.internalError(w, "api: start canary: record", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	rel, _, _ = rt.canaries.GetCanaryRelease(r.Context(), name)
	writeJSON(w, http.StatusCreated, rt.toCanaryResource(r.Context(), name, rel, true))
}

func (rt *Router) canaryBlockReason(ctx context.Context, svc store.DesiredService) string {
	switch {
	case svc.HostPort != nil, len(svc.PublishedPorts) > 0:
		return "an app with a pinned or published host port cannot run two releases at once"
	case svc.Replicas > 1:
		return "canary needs a single-replica app"
	case svc.ImageIDRef != "":
		return "built from source, canary supports image apps"
	case svc.Suspended:
		return "app is stopped"
	case rt.hasRunningDeployAttempt(ctx, svc.Name):
		return "a deploy is already running"
	}
	if frozen, reason := rt.freezeStatus(ctx, svc.Name); frozen {
		return "deploy freeze active: " + reason
	}
	return ""
}

// handleSetCanaryWeight sets the share by hand. On a staged canary a weight of
// 0 pauses the automation; any other value is overwritten at the next stage.
func (rt *Router) handleSetCanaryWeight(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req canaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Weight == nil || !validCanaryWeight(*req.Weight) {
		writeError(w, http.StatusBadRequest, "weight must be between 0 and 99 (0 pauses the canary)")
		return
	}
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set canary weight: load", err, slog.String("name", name))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	c.Weight = *req.Weight
	if err := rt.canaries.SetCanaryRelease(r.Context(), c); err != nil {
		rt.internalError(w, "api: set canary weight: save", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.toCanaryResource(r.Context(), name, c, true))
}

// handleAbortCanary removes the canary clone; all traffic returns to the stable release.
func (rt *Router) handleAbortCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: abort canary: load", err, slog.String("name", name))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	rt.recordCanaryOutcome(r.Context(), c, store.CanaryOutcomeAborted, "aborted by an operator")
	if err := rt.removeCanary(r.Context(), name); err != nil {
		rt.internalError(w, "api: abort canary", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toCanaryResource(r.Context(), name, store.CanaryRelease{}, false))
}

// handlePromoteCanary deploys the canary image to the stable app through the
// normal deploy path (readiness gate, rollback target), then removes the clone.
func (rt *Router) handlePromoteCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: promote canary: load", err, slog.String("name", name))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	if err := rt.promoteCanary(r.Context(), c, "canary promote"); err != nil {
		var refused canaryRefusedError
		if errors.As(err, &refused) {
			writeError(w, http.StatusConflict, refused.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rt.toCanaryResource(r.Context(), name, store.CanaryRelease{}, false))
}

type canaryRefusedError string

func (e canaryRefusedError) Error() string { return string(e) }

// promoteCanary is idempotent: when the stable app already runs the canary
// image (an earlier attempt deployed but failed to clean up) it only cleans up.
func (rt *Router) promoteCanary(ctx context.Context, c store.CanaryRelease, reason string) error {
	name := c.ServiceName
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return fmt.Errorf("load app: %w", err)
	}
	if svc.Image != c.Image {
		if why := rt.imageUpdateSkipReason(ctx, *svc); why != "" {
			return canaryRefusedError(why)
		}
		trigger := rt.imageTrigger(ctx, *svc, true, reason)
		trigger.Source = store.DeployAttemptSourceImage
		trigger.Ordered, trigger.Sequencer = nil, nil
		updated, err := trigger.Deploy(ctx, *svc, c.Image)
		if err != nil {
			return err
		}
		if rt.deployNotifier != nil {
			rt.deployNotifier.Dispatch(ctx, resourceIDForApp(name), alerting.DeployOutcome{AppName: name, Image: updated.Image, Succeeded: true})
		}
	}
	rt.recordCanaryOutcome(ctx, c, store.CanaryOutcomePromoted, reason)
	if err := rt.removeCanary(ctx, name); err != nil {
		return fmt.Errorf("remove canary after promote: %w", err)
	}
	return nil
}

func (rt *Router) recordCanaryOutcome(ctx context.Context, c store.CanaryRelease, outcome, reason string) {
	err := rt.canaries.SetCanaryOutcome(ctx, store.CanaryOutcome{ServiceName: c.ServiceName, Outcome: outcome, Reason: reason, Image: c.Image})
	if err != nil {
		rt.logger.Warn("api: record canary outcome failed", slog.String("name", c.ServiceName), slog.String("error", err.Error()))
	}
}

// removeCanary deletes the clone before the release row, so a failure leaves
// the row in place and a retry (operator or reconciler) finishes the job.
func (rt *Router) removeCanary(ctx context.Context, name string) error {
	if err := rt.deleteCanaryClone(ctx, store.CanaryServiceName(name)); err != nil {
		return err
	}
	if err := rt.canaries.DeleteCanaryRelease(ctx, name); err != nil {
		return err
	}
	rt.nudgeReconciler()
	return nil
}

func (rt *Router) deleteCanaryClone(ctx context.Context, cloneName string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cloneRollbackTimeout)
	defer cancel()
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, cloneName); err != nil {
			return fmt.Errorf("delete canary secrets: %w", err)
		}
	}
	teardownErr, err := rt.deleteApp(ctx, cloneName)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		return fmt.Errorf("delete canary clone: %w", err)
	}
	if teardownErr != nil {
		rt.logger.Warn("api: canary clone teardown incomplete", slog.String("name", cloneName), slog.String("error", teardownErr.Error()))
	}
	return nil
}

// CanaryActions returns the side effects the canary reconciler applies on a
// decision: a promote through the normal deploy path, or a rollback that
// records why and removes the canary.
func (rt *Router) CanaryActions() canary.Actions { return canaryActions{rt} }

type canaryActions struct{ rt *Router }

func (a canaryActions) Promote(ctx context.Context, rel store.CanaryRelease) error {
	return a.rt.promoteCanary(ctx, rel, "canary auto promote")
}

func (a canaryActions) Rollback(ctx context.Context, rel store.CanaryRelease, reason string) error {
	rt := a.rt
	rt.recordCanaryOutcome(ctx, rel, store.CanaryOutcomeRolledBack, reason)
	if err := rt.removeCanary(ctx, rel.ServiceName); err != nil {
		return err
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(rel.ServiceName), alerting.DeployOutcome{
			AppName: rel.ServiceName, Image: rel.Image, Error: "canary rolled back automatically: " + reason,
		})
	}
	return nil
}
