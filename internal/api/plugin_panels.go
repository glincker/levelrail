package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

const panelActionBodyLimit = 4 << 10

type pluginPanelEntry struct {
	Plugin    string       `json:"plugin"`
	Publisher string       `json:"publisher"`
	Panel     plugin.Panel `json:"panel"`
}

type panelActionRequest struct {
	AppID string `json:"app_id,omitempty"`
}

// handleListPluginPanels serves GET /api/v1/plugin-panels?slot=. Only enabled
// panel providers appear, and only panels the operator approved abilities for.
func (rt *Router) handleListPluginPanels(w http.ResponseWriter, r *http.Request) {
	m := rt.plugins
	if m == nil {
		writeJSON(w, http.StatusOK, []pluginPanelEntry{})
		return
	}
	slot := r.URL.Query().Get("slot")
	if slot == "" {
		slot = plugin.SlotPlugins
	}
	recs, err := m.List(r.Context())
	if err != nil {
		rt.writePluginError(w, "list panels", err)
		return
	}
	out := []pluginPanelEntry{}
	for _, rec := range recs {
		if rec.Kind != plugin.KindPanelProvider || rec.State != plugin.StateEnabled {
			continue
		}
		man, err := plugin.ParseManifest([]byte(rec.Manifest))
		if err != nil {
			continue
		}
		for _, p := range man.SlotPanels(slot) {
			out = append(out, pluginPanelEntry{Plugin: rec.Name, Publisher: rec.Publisher, Panel: p})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePluginPanelData serves GET /api/v1/plugins/{name}/panels/{panel}/data.
func (rt *Router) handlePluginPanelData(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	scope, ok := rt.panelScope(w, r, r.URL.Query().Get(plugin.ScopeKeyAppID))
	if !ok {
		return
	}
	view, err := m.PanelData(r.Context(), r.PathValue("name"), r.PathValue("panel"), scope)
	if err != nil {
		rt.writePanelError(w, "data", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handlePluginPanelAction serves POST /api/v1/plugins/{name}/panels/{panel}/actions/{action}.
func (rt *Router) handlePluginPanelAction(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req panelActionRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBodyLimit(r, &req, panelActionBodyLimit); err != nil {
			writeError(w, http.StatusBadRequest, "request body must be JSON")
			return
		}
	}
	scope, ok := rt.panelScope(w, r, req.AppID)
	if !ok {
		return
	}
	res, err := m.PanelAction(r.Context(), r.PathValue("name"), r.PathValue("panel"), r.PathValue("action"), scope)
	if err != nil {
		rt.writePanelError(w, "action", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// panelScope authorizes the app a panel call is scoped to before it is handed to the plugin.
func (rt *Router) panelScope(w http.ResponseWriter, r *http.Request, appID string) (map[string]string, bool) {
	scope := map[string]string{}
	if appID == "" {
		return scope, true
	}
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "app not found"), appRef(appID)) {
		return nil, false
	}
	_, found, err := rt.lookupAppResource(r.Context(), appID)
	if err != nil {
		rt.internalError(w, "api: plugin panel app lookup", err)
		return nil, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	scope[plugin.ScopeKeyAppID] = appID
	return scope, true
}

func (rt *Router) writePanelError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, plugin.ErrNoPanel), errors.Is(err, plugin.ErrNotPanel):
		writeError(w, http.StatusNotFound, "no such plugin panel")
	case errors.Is(err, plugin.ErrUnavailable), errors.Is(err, plugin.ErrCircuitOpen):
		writeError(w, http.StatusServiceUnavailable, "the plugin is not running or is paused after repeated failures")
	case errors.Is(err, plugin.ErrBusy):
		writeError(w, http.StatusTooManyRequests, "the plugin is busy, retry shortly")
	case errors.Is(err, plugin.ErrTooLarge):
		writeError(w, http.StatusBadGateway, "the plugin's response was larger than the allowed size")
	case errors.Is(err, plugin.ErrBadData):
		writeError(w, http.StatusBadGateway, "the plugin returned data that does not match its panel")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "the plugin did not answer in time")
	case errors.Is(err, plugin.ErrCallFailed):
		rt.logger.Warn("api: plugin panel call failed", slog.String("op", what), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the plugin call failed")
	default:
		rt.writePluginError(w, "panel "+what, err)
	}
}
