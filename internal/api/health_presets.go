package api

import (
	"net/http"

	"github.com/glincker/levelrail/internal/healthpreset"
)

// handleGetHealthPresets handles GET /api/v1/health-presets: the shortcut paths and timing the
// CLI and the dashboard share.
func (rt *Router) handleGetHealthPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthpreset.Current())
}
