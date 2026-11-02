package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type autoHostStore interface {
	GetAppAutoHostOptOut(ctx context.Context, app string) (bool, error)
	SetAppAutoHostOptOut(ctx context.Context, app string, optOut bool) error
}

type autoHostResource struct {
	// Enabled is true when the app gets an automatic hostname and certificate (the default).
	Enabled bool `json:"enabled"`
	// Hostname is the automatic hostname now in use, empty when opted out or none can be formed.
	Hostname string `json:"hostname,omitempty"`
}

type autoHostUpdate struct {
	Enabled *bool `json:"enabled"`
}

func (rt *Router) autoHostState(ctx context.Context, st autoHostStore, app string) (autoHostResource, error) {
	off, err := st.GetAppAutoHostOptOut(ctx, app)
	if err != nil {
		return autoHostResource{}, err
	}
	out := autoHostResource{Enabled: !off}
	if !off {
		out.Hostname, _ = rt.autoHost(ctx, app)
	}
	return out, nil
}

func (rt *Router) autoHostApp(w http.ResponseWriter, r *http.Request) (autoHostStore, string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "api: auto host: load app failed", err, slog.String("name", name))
		}
		return nil, "", false
	}
	st, ok := rt.ingressSettings.(autoHostStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "automatic hostname settings are not available")
		return nil, "", false
	}
	return st, name, true
}

// handleGetAppAutoHost handles GET /api/v1/apps/{name}/auto-host.
func (rt *Router) handleGetAppAutoHost(w http.ResponseWriter, r *http.Request) {
	st, name, ok := rt.autoHostApp(w, r)
	if !ok {
		return
	}
	res, err := rt.autoHostState(r.Context(), st, name)
	if err != nil {
		rt.internalError(w, "api: get auto host failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetAppAutoHost handles PUT /api/v1/apps/{name}/auto-host: enabled false stops the automatic hostname and its certificate.
func (rt *Router) handleSetAppAutoHost(w http.ResponseWriter, r *http.Request) {
	st, name, ok := rt.autoHostApp(w, r)
	if !ok {
		return
	}
	var body autoHostUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "body must be {\"enabled\": true|false}")
		return
	}
	if err := st.SetAppAutoHostOptOut(r.Context(), name, !*body.Enabled); err != nil {
		rt.internalError(w, "api: set auto host failed", err, slog.String("name", name))
		return
	}
	title := "Automatic hostname turned on"
	if !*body.Enabled {
		title = "Automatic hostname and certificate turned off"
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: title})
	rt.nudgeReconciler()
	res, err := rt.autoHostState(r.Context(), st, name)
	if err != nil {
		rt.internalError(w, "api: get auto host failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
