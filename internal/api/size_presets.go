package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// handleSizePresets handles GET /api/v1/size-presets: the live preset table.
func (rt *Router) handleSizePresets(w http.ResponseWriter, r *http.Request) {
	raw := ""
	if rt.runtimeSettings != nil {
		raw = rt.runtimeSettings.String(r.Context(), runtimesettings.KeyResourcesSizePresets)
	}
	presets, err := runtimesettings.ParseSizePresets(raw)
	if err != nil {
		presets, _ = runtimesettings.ParseSizePresets(runtimesettings.DefaultSizePresets)
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": presets})
}
