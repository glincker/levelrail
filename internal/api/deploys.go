package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applicationControllerName mirrors
// internal/reconcile/application.Controller.Name()'s "application/" +
// serviceName convention. Duplicated as a plain string format here
// rather than imported, because importing that package would pull
// internal/docker's whole runtime dependency into internal/api just for
// one string format; if that naming convention ever changes, both call
// sites need updating together.
func applicationControllerName(appName string) string {
	return "application/" + appName
}

type deployTriggerRequest struct {
	Image   string `json:"image"`
	Confirm bool   `json:"confirm,omitempty"`
}

// handleTriggerDeploy handles POST /api/v1/apps/{name}/deploys. It
// points the app's desired image at a new tag; the application
// controller's next reconcile (once main.go wires one for this app, see
// router.go's package doc comment) is what actually creates a container
// from it. This is exactly the mechanism used for
// rollback ("pointing desired.Image back at an older tag and
// reconciling converges to it the same way any other redeploy does"),
// run forward with a newer tag instead of an older one. The build that
// produces that tag (1.4/1.5) is not this endpoint's job. If name is
// tagged with a protected environment, confirm: true is required
// (requireEnvironmentConfirmation, environments.go).
func (rt *Router) handleTriggerDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: trigger deploy: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req deployTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}

	if !rt.requireEnvironmentConfirmation(r.Context(), w, existing.EnvironmentID, req.Confirm) {
		return
	}

	// deploy.TriggerImageDeploy is the same shared save+record+nudge path
	// internal/alerting's crashloop auto-rollback (MaybeAutoRollback)
	// drives too: a manual rollback and an automatic one converge through
	// identical code, not two divergent ones. rt.reconcileNudger already
	// satisfies deploy.ReconcileNudger (both declare exactly Nudge()).
	updated, err := deploy.TriggerImageDeploy(r.Context(), deployTriggerImageStore{rt}, rt.reconcileNudger, *existing, req.Image, store.DeployAttemptSourceImage, rt.logger)
	if err != nil {
		rt.logger.Error("api: trigger deploy failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(r.Context(), resourceIDForApp(name), alerting.DeployOutcome{
			AppName: name, Image: req.Image, Succeeded: true,
		})
	}

	writeJSON(w, http.StatusAccepted, toAppResource(updated))
}

// deployTriggerImageStore adapts rt.apps and rt.deployAttempts, two
// separately-typed Router fields, to the single deploy.ImageDeployStore
// surface deploy.TriggerImageDeploy needs: Go interfaces don't compose
// across two independent fields without an adapter like this one.
type deployTriggerImageStore struct{ rt *Router }

func (s deployTriggerImageStore) SaveDesiredService(ctx context.Context, svc store.DesiredService) error {
	return s.rt.apps.SaveDesiredService(ctx, svc)
}

func (s deployTriggerImageStore) SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error {
	return s.rt.deployAttempts.SaveDeployAttempt(ctx, a)
}

func (s deployTriggerImageStore) FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error {
	return s.rt.deployAttempts.FinishDeployAttempt(ctx, id, status, finishedAt, errMsg)
}

// setAutoRollbackRequest is PUT /api/v1/apps/{name}/auto-rollback's body.
type setAutoRollbackRequest struct {
	Enabled bool `json:"enabled"`
}

// autoRollbackSettingResource is both GET and PUT
// /api/v1/apps/{name}/auto-rollback's response.
type autoRollbackSettingResource struct {
	Enabled bool `json:"enabled"`
}

// handleGetAutoRollback handles GET /api/v1/apps/{name}/auto-rollback:
// the current value of store.DesiredService.AutoRollbackOnCrashloop, so
// the dashboard toggle (and "apps auto-rollback status") has something to
// read on load without waiting for a PUT.
func (rt *Router) handleGetAutoRollback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get auto-rollback: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, autoRollbackSettingResource{Enabled: svc.AutoRollbackOnCrashloop})
}

// handleSetAutoRollback handles PUT /api/v1/apps/{name}/auto-rollback:
// opts name into (or out of) automatic rollback on a crashloop, the
// per-app switch internal/alerting.MaybeAutoRollback checks before
// rolling back a firing KindCrashloop rule's app. Off by default
// (migrations/0106_service_auto_rollback_on_crashloop.sql), matching
// every other opt-in feature toggle in this codebase.
func (rt *Router) handleSetAutoRollback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAutoRollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.apps.SetServiceAutoRollbackOnCrashloop(r.Context(), name, req.Enabled); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.logger.Error("api: set auto-rollback failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, autoRollbackSettingResource(req))
}

// setDesiredImage points existing's image at image and saves it: the
// same mutation handleTriggerDeploy and handlePromoteApp (promote.go)
// both drive, clearing EnvDirty the same way RestartService already
// does for a real redeploy.
func (rt *Router) setDesiredImage(ctx context.Context, existing store.DesiredService, image string) (store.DesiredService, error) {
	updated := existing
	updated.Image = image
	updated.EnvDirty = false
	if err := rt.apps.SaveDesiredService(ctx, updated); err != nil {
		return store.DesiredService{}, err
	}
	return updated, nil
}

// recordPlainDeployAttempt records a deploy_attempts row for the plain
// image-tag path (create, update, and manual redeploy/rollback all share
// it): it deserves history and rollback treatment just like the two
// build-triggering paths, even with no build step to wait on. svc is the
// service's desired state as of this trigger (already carrying image, in
// every real call site), snapshotted onto the row for
// GET .../deploys/compare. See recordInstantDeployAttempt for the
// mechanics.
func (rt *Router) recordPlainDeployAttempt(ctx context.Context, svc store.DesiredService, image string) {
	rt.recordInstantDeployAttempt(ctx, svc, image, store.DeployAttemptSourceImage)
}

// recordInstantDeployAttempt saves and immediately finishes (as
// succeeded) a deploy_attempts row for a trigger with no build step and
// no async work to wait on: the caller's own desired-state write has
// already fully completed by the time this runs. source distinguishes
// the plain image-tag path (recordPlainDeployAttempt) from the
// compose/template fan-out path (apps_compose.go's handleDeployCompose),
// which calls this directly with DeployAttemptSourceCompose once per
// service.
//
// A failure here (minting the ID, or either store call) is logged, not
// returned as the caller's own error: the desired-state write already
// succeeded, so a history-tracking hiccup must never turn an otherwise-
// successful deploy trigger into a client-visible failure.
func (rt *Router) recordInstantDeployAttempt(ctx context.Context, svc store.DesiredService, image, source string) {
	serviceName := svc.Name
	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: record deploy attempt: mint id failed", slog.String("error", err.Error()), slog.String("name", serviceName))
		return
	}
	now := time.Now()
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: serviceName, Image: image,
		Source: source,
		Status: store.DeployAttemptStatusRunning, StartedAt: now,
		Snapshot: store.NewDeployAttemptSnapshot(svc),
	}); err != nil {
		rt.logger.Error("api: record deploy attempt: save failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	if err := rt.deployAttempts.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		rt.logger.Error("api: record deploy attempt: finish failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(serviceName), alerting.DeployOutcome{
			AppName: serviceName, Image: image, Succeeded: true,
		})
	}
}

// handleDeployHistory handles GET /api/v1/apps/{name}/deploys. It
// surfaces the application controller's stored reconcile conditions for
// this app, per the reconciler contract that every reconcile emits a
// status condition with a reason string, stored and shown in the UI.
// This is current reconcile status, not a log of past deploy attempts:
// internal/store.UpsertConditions only ever persists the latest
// condition per (controller, condition type) pair, it has no history
// table. Router.go's package doc comment tracks that as a real gap
// rather than something this handler fakes with an invented log format.
func (rt *Router) handleDeployHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: deploy history: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	conditions, err := rt.deploys.GetConditions(r.Context(), applicationControllerName(name))
	if err != nil {
		rt.logger.Error("api: deploy history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, conditions)
}
