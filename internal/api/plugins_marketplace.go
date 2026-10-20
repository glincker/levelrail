package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

// SetPluginMarketplace enables the plugin marketplace routes.
func (rt *Router) SetPluginMarketplace(m *plugin.Marketplace) { rt.marketplace = m }

func (rt *Router) marketplaceService(w http.ResponseWriter) (*plugin.Marketplace, bool) {
	if rt.marketplace == nil {
		writeError(w, http.StatusServiceUnavailable, "the plugin marketplace is not available on this control plane (it needs the plugins feature)")
		return nil, false
	}
	return rt.marketplace, true
}

func (rt *Router) writeMarketplaceError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, plugin.ErrEntryNotFound), errors.Is(err, plugin.ErrSourceUnknown):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, plugin.ErrChanged), errors.Is(err, plugin.ErrSourceExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, plugin.ErrNoRuntime):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, plugin.ErrIndexSignature), errors.Is(err, plugin.ErrIndexReplay), errors.Is(err, plugin.ErrIndexTooLarge), errors.Is(err, plugin.ErrIndexFormat):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		rt.writePluginError(w, "marketplace "+what, err)
	}
}

type marketplacePreviewRequest struct {
	SourceID string `json:"source_id"`
	ID       string `json:"id"`
}

type marketplaceRefreshRequest struct {
	SourceID string `json:"source_id"`
}

type marketplaceSourceRequest struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	RootKey string `json:"root_public_key"`
}

// handleMarketplaceCatalog serves GET /api/v1/plugin-marketplace from the cached, verified indexes.
func (rt *Router) handleMarketplaceCatalog(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	cat, err := m.Catalog(r.Context(), false)
	if err != nil {
		rt.writeMarketplaceError(w, "catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, cat)
}

// handleMarketplaceRefresh serves POST /api/v1/plugin-marketplace/refresh, the Re-check button.
func (rt *Router) handleMarketplaceRefresh(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	var req marketplaceRefreshRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
			writeError(w, http.StatusBadRequest, "request body must be JSON")
			return
		}
	}
	if _, err := m.Refresh(r.Context(), req.SourceID); err != nil {
		rt.writeMarketplaceError(w, "refresh", err)
		return
	}
	cat, err := m.Catalog(r.Context(), false)
	if err != nil {
		rt.writeMarketplaceError(w, "refresh", err)
		return
	}
	writeJSON(w, http.StatusOK, cat)
}

// handleMarketplacePreview serves POST /api/v1/plugin-marketplace/preview, the confirm sheet. It writes nothing.
func (rt *Router) handleMarketplacePreview(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	var req marketplacePreviewRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	pv, err := m.Preview(r.Context(), req.SourceID, req.ID)
	if err != nil {
		rt.writeMarketplaceError(w, "preview", err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

// handleMarketplaceInstall serves POST /api/v1/plugin-marketplace/install. The body repeats what the sheet showed and the server re-verifies all of it.
func (rt *Router) handleMarketplaceInstall(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	var req plugin.MarketInstallRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	res, err := m.Install(r.Context(), req, rt.pluginActor(r))
	if err != nil {
		rt.writeMarketplaceError(w, "install", err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Plugin     pluginResource `json:"plugin"`
		Action     string         `json:"action"`
		TrustAdded bool           `json:"trust_added"`
	}{rt.toPluginResource(r.Context(), rt.plugins, res.Plugin), res.Action, res.TrustAdded})
}

// handleMarketplaceSources serves GET /api/v1/plugin-marketplace/sources.
func (rt *Router) handleMarketplaceSources(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	src, err := m.Sources(r.Context())
	if err != nil {
		rt.writeMarketplaceError(w, "sources", err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// handleMarketplaceAddSource serves POST /api/v1/plugin-marketplace/sources.
func (rt *Router) handleMarketplaceAddSource(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	var req marketplaceSourceRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	s, err := m.AddSource(r.Context(), req.Name, req.URL, req.RootKey, rt.pluginActor(r))
	if err != nil {
		rt.writeMarketplaceError(w, "add source", err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// handleMarketplaceRemoveSource serves DELETE /api/v1/plugin-marketplace/sources/{id}.
func (rt *Router) handleMarketplaceRemoveSource(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.marketplaceService(w)
	if !ok {
		return
	}
	if err := m.RemoveSource(r.Context(), r.PathValue("id")); err != nil {
		rt.writeMarketplaceError(w, "remove source", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
