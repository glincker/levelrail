package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

type appProtectionStore interface {
	GetAppProtection(ctx context.Context, app string) (*store.AppProtection, error)
	SetAppProtection(ctx context.Context, app, reason, by string) error
	ClearAppProtection(ctx context.Context, app string) (bool, error)
}

type appProtectionResource struct {
	Protected  bool                 `json:"protected"`
	Protection *store.AppProtection `json:"protection,omitempty"`
}

// refuseIfProtected writes 423 and returns true when the app is protected.
// A failed lookup refuses too, so a store hiccup never lets a write through.
func (rt *Router) refuseIfProtected(w http.ResponseWriter, r *http.Request, name string) bool {
	st, ok := rt.apps.(appProtectionStore)
	if !ok {
		return false
	}
	p, err := st.GetAppProtection(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: check app protection failed", err, slog.String("name", name))
		return true
	}
	if p == nil {
		return false
	}
	writeError(w, http.StatusLocked, "app is protected: "+p.Reason+"; unprotect it first (CLI: apps unprotect "+name+")")
	return true
}

func (rt *Router) protectionStore(w http.ResponseWriter) (appProtectionStore, bool) {
	st, ok := rt.apps.(appProtectionStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "app protection is not available")
	}
	return st, ok
}

func (rt *Router) writeProtection(w http.ResponseWriter, r *http.Request, st appProtectionStore, name string) {
	p, err := st.GetAppProtection(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get app protection failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, appProtectionResource{Protected: p != nil, Protection: p})
}

// handleGetAppProtection handles GET /api/v1/apps/{name}/protection; reads stay open.
func (rt *Router) handleGetAppProtection(w http.ResponseWriter, r *http.Request) {
	if st, ok := rt.protectionStore(w); ok {
		rt.writeProtection(w, r, st, r.PathValue("name"))
	}
}

// handlePutAppProtection handles PUT /api/v1/apps/{name}/protection: a reason is required.
func (rt *Router) handlePutAppProtection(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.protectionStore(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"reason\": \"...\"} with a non-empty reason")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if err := st.SetAppProtection(r.Context(), name, reason, rt.eventActor(r)); err != nil {
		rt.internalError(w, "api: protect app failed", err, slog.String("name", name))
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: "App protected", Detail: reason})
	rt.writeProtection(w, r, st, name)
}

// handleDeleteAppProtection handles DELETE /api/v1/apps/{name}/protection and audits the unlock.
func (rt *Router) handleDeleteAppProtection(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.protectionStore(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	had, err := st.ClearAppProtection(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: unprotect app failed", err, slog.String("name", name))
		return
	}
	if had {
		rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: "App unprotected"})
	}
	rt.writeProtection(w, r, st, name)
}
