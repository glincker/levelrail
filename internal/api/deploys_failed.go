package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const defaultFailedDeploysWindow = 24 * time.Hour

type failedDeployResource struct {
	deployAttemptResource
	LastGoodImage string `json:"last_good_image,omitempty"`
}

// handleListFailedDeploys handles GET /api/v1/deploys/failed?since=24h:
// apps whose latest deploy attempt failed inside the window, one per app.
func (rt *Router) handleListFailedDeploys(w http.ResponseWriter, r *http.Request) {
	window := defaultFailedDeploysWindow
	if raw := r.URL.Query().Get("since"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "since must be a positive duration such as 24h")
			return
		}
		window = d
	}

	failed, err := rt.deployAttempts.ListFailedDeploysSince(r.Context(), time.Now().Add(-window))
	if err != nil {
		rt.logger.Error("api: list failed deploys failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list failed deploys: visibility", err)
		return
	}
	out := make([]failedDeployResource, 0, len(failed))
	for _, f := range failed {
		if !canSee(f.Attempt.ServiceName) {
			continue
		}
		out = append(out, failedDeployResource{toDeployAttemptResource(f.Attempt), f.LastGoodImage})
	}
	writeJSON(w, http.StatusOK, out)
}

// appVisibilityFilter returns a predicate reporting whether the caller can
// read an app by name, so rows for since-deleted apps are still judged by IAM.
func (rt *Router) appVisibilityFilter(r *http.Request) (func(app string) bool, error) {
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		return nil, fmt.Errorf("resolve caller visibility: %w", err)
	}
	if !filtered {
		return func(string) bool { return true }, nil
	}
	return canRead, nil
}
