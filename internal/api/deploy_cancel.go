package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

// buildCancelRegistry tracks the context.CancelFunc for every in-progress
// manual build/deploy started by handleTriggerBuild (builds.go), keyed by
// its deploy_attempts row ID. Purely in-memory: a control plane restart
// loses the registry, but FailOrphanedDeployAttempts already marks any
// still-"running" row failed on the next startup regardless.
//
// Only handleTriggerBuild registers into this. The git-push webhook
// (internal/webhook) and the plain image-tag deploy/rollback path
// (deploys.go) mint their own deploy_attempts rows through the same
// beginBuildDeployAttempt/recordPlainDeployAttempt helpers but never
// register a cancel func here, so POST .../deploys/{id}/cancel for either
// of those returns a 409, not a real cancellation. That gap is deliberate
// scope for this endpoint, not an oversight: see handleCancelDeploy's own
// doc comment.
type buildCancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newBuildCancelRegistry() *buildCancelRegistry {
	return &buildCancelRegistry{cancels: make(map[string]context.CancelFunc)}
}

func (r *buildCancelRegistry) register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[id] = cancel
}

func (r *buildCancelRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

// cancel calls id's registered cancel func and reports whether one was
// found. False means either id was never registered or has already been
// removed (register/remove above, both called from handleTriggerBuild's
// own goroutine).
func (r *buildCancelRegistry) cancel(id string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// cancelDeployResponse is POST .../deploys/{attemptId}/cancel's success
// body: the cancel signal has been sent, not that the build has actually
// stopped yet, since rt.builder.Deploy notices ctx cancellation
// asynchronously.
type cancelDeployResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// handleCancelDeploy handles
// POST /api/v1/apps/{name}/deploys/{attemptId}/cancel: cancels an
// in-progress manual build/deploy started by handleTriggerBuild
// (builds.go), the only trigger path that registers into rt.buildCancels.
//
// Known, deliberate gap: a webhook-triggered deploy
// (internal/webhook.Handler) and the plain image-tag deploy/rollback path
// (deploys.go's handleTriggerDeploy) are not cancelable through this
// route. Both mint their own deploy_attempts row (Source "webhook" or
// "image") but never register a cancel func, so a caller pointing this at
// either one gets a 409 naming the actual source rather than a silent
// no-op.
func (rt *Router) handleCancelDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	attemptID := r.PathValue("attemptId")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.logger.Error("api: cancel deploy: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), attemptID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: cancel deploy: load attempt failed", slog.String("error", err.Error()), slog.String("attempt_id", attemptID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Attempt IDs are globally unique but the route is app-scoped: a
	// mismatch here is treated as not-found, not a cross-app leak, the
	// same convention handleDeployLogStream already establishes.
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}

	if !rt.buildCancels.cancel(attemptID) {
		if attempt.FinishedAt != nil {
			writeError(w, http.StatusConflict, fmt.Sprintf("deploy attempt %q already finished with status %q", attemptID, attempt.Status))
			return
		}
		writeError(w, http.StatusConflict, fmt.Sprintf("deploy attempt %q cannot be canceled: source %q is not a cancelable manual build trigger", attemptID, attempt.Source))
		return
	}

	writeJSON(w, http.StatusOK, cancelDeployResponse{ID: attemptID, Status: "canceling"})
}
