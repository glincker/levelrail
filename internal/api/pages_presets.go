package api

import (
	"log/slog"
	"net/http"

	"github.com/glincker/levelrail/internal/pages"
)

// pagesPresetBody is the PUT body: an empty preset clears the choice.
type pagesPresetBody struct {
	Preset string `json:"preset"`
}

type pagesPresetResource struct {
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

// handleListPagesPresets handles GET /api/v1/pages/presets.
func (rt *Router) handleListPagesPresets(w http.ResponseWriter, _ *http.Request) {
	all, err := pages.All()
	if err != nil {
		rt.internalError(w, "api: pages presets: load", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": all})
}

// handleGetStaticSitePreset handles GET /api/v1/static-sites/{name}/preset.
func (rt *Router) handleGetStaticSitePreset(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	s, _, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: pages preset: read", err, slog.String("name", site.Name))
		return
	}
	writeJSON(w, http.StatusOK, pagesPresetResource{Name: site.Name, Preset: s.Preset})
}

// handlePutStaticSitePreset handles PUT /api/v1/static-sites/{name}/preset and
// re-publishes the live rules so the headers apply without a redeploy.
func (rt *Router) handlePutStaticSitePreset(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	var body pagesPresetBody
	if err := decodeJSONBodyLimit(r, &body, maxStaticRoutesBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Preset != "" {
		if _, found := pages.Get(body.Preset); !found {
			writeError(w, http.StatusUnprocessableEntity, "unknown preset")
			return
		}
	}
	s, ui, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: pages preset: read", err, slog.String("name", site.Name))
		return
	}
	s.Preset = body.Preset
	if err := st.SaveStaticSiteSettings(r.Context(), site.Name, s); err != nil {
		rt.internalError(w, "api: pages preset: save", err, slog.String("name", site.Name))
		return
	}
	if err := rt.publishStaticUI(site, s, ui, nil); err != nil {
		rt.internalError(w, "api: pages preset: apply to live site", err, slog.String("name", site.Name))
		return
	}
	rt.auditGitSetting(r, site.Name, "static_site.preset_set", map[string]any{"site": site.Name, "preset": body.Preset})
	writeJSON(w, http.StatusOK, pagesPresetResource{Name: site.Name, Preset: s.Preset})
}
