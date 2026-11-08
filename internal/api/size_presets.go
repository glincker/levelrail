package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func (rt *Router) registerSizePresetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/size-presets", rt.requireAbility(AbilityRead, rt.handleListSizePresets))
}

type sizePresetsResponse struct {
	Presets []runtimesettings.SizePreset `json:"presets"`
}

// handleListSizePresets handles GET /api/v1/size-presets. A saved value that no longer parses
// falls back to the registry default so the list is never empty.
func (rt *Router) handleListSizePresets(w http.ResponseWriter, r *http.Request) {
	raw := rt.runtimeSettings.String(r.Context(), runtimesettings.KeySizePresets)
	presets, err := runtimesettings.ParseSizePresets(raw)
	if err != nil {
		presets, _ = runtimesettings.ParseSizePresets(runtimesettings.DefaultSizePresets)
	}
	writeJSON(w, http.StatusOK, sizePresetsResponse{Presets: presets})
}
