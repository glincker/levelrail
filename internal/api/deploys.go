package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
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
	Image string `json:"image"`
}

// handleTriggerDeploy handles POST /api/v1/apps/{name}/deploys. It
// points the app's desired image at a new tag; the application
// controller's next reconcile (once main.go wires one for this app, see
// router.go's package doc comment) is what actually creates a container
// from it. This is exactly the mechanism TASKS.md 1.3 documents for
// rollback ("pointing desired.Image back at an older tag and
// reconciling converges to it the same way any other redeploy does"),
// run forward with a newer tag instead of an older one. The build that
// produces that tag (1.4/1.5) is not this endpoint's job.
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

	updated := *existing
	updated.Image = req.Image
	if err := rt.apps.SaveDesiredService(r.Context(), updated); err != nil {
		rt.logger.Error("api: trigger deploy failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.recordPlainDeployAttempt(r.Context(), name, req.Image)

	writeJSON(w, http.StatusAccepted, toAppResource(updated))
}

// recordPlainDeployAttempt saves this task's judgment call for the
// plain image-tag path: it deserves a deploy_attempts row for history
// and rollback purposes just like the two build-triggering paths, even
// though it has no real build step (no Dockerfile build happens, see
// handleTriggerDeploy's own doc comment) and so nothing worth persisting
// to the deploy-log store. The row is saved and immediately finished as
// succeeded in one call, not started as "running" and finished later by
// a background build: SaveDesiredService above has already fully
// completed the only work this path does by the time this runs.
//
// A failure here (minting the ID, or either store call) is logged, not
// returned as this handler's own error: the desired-state write already
// succeeded by this point, so a history-tracking hiccup must not turn an
// otherwise-successful deploy trigger into a client-visible failure, the
// same "must never block the real operation" choice
// deploy.Pipeline.deployDockerfile's own metrics-recording already
// makes.
//
// This path always finishes as succeeded (recordPlainDeployAttempt's own
// doc comment: no real build step happens here), so the deploy-outcome
// notification it dispatches, when rt.deployNotifier is configured, is
// always a success notification for this trigger path. A future build
// failure elsewhere in this same attempt's lifecycle isn't possible
// today, matching FinishDeployAttempt's own hardcoded
// DeployAttemptStatusSucceeded call just below.
func (rt *Router) recordPlainDeployAttempt(ctx context.Context, serviceName, image string) {
	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: trigger deploy: mint deploy attempt id failed", slog.String("error", err.Error()), slog.String("name", serviceName))
		return
	}
	now := time.Now()
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: serviceName, Image: image,
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusRunning, StartedAt: now,
	}); err != nil {
		rt.logger.Error("api: trigger deploy: save deploy attempt failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	if err := rt.deployAttempts.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		rt.logger.Error("api: trigger deploy: finish deploy attempt failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
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
