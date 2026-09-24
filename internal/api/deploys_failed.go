package api

import (
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
	out := make([]failedDeployResource, 0, len(failed))
	for _, f := range failed {
		out = append(out, failedDeployResource{toDeployAttemptResource(f.Attempt), f.LastGoodImage})
	}
	writeJSON(w, http.StatusOK, out)
}
