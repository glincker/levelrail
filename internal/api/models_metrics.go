package api

import (
	"net/http"
	"strings"
	"time"
)

// handleModelEngineMetrics handles GET /api/v1/models/{name}/engine-metrics?since=1h.
func (rt *Router) handleModelEngineMetrics(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	window := time.Hour
	if v := strings.TrimSpace(r.URL.Query().Get("since")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "since must be a positive duration such as 1h or 24h")
			return
		}
		window = d
	}
	rep, err := rt.models.EngineMetrics(r.Context(), r.PathValue("name"), window)
	if err != nil {
		rt.writeModelError(w, "read model engine metrics", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
