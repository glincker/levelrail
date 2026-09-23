package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// appHealthResource is GET/PUT /api/v1/apps/{name}/health's body.
type appHealthResource struct {
	Name   string               `json:"name"`
	Health *store.ServiceHealth `json:"health"`
}

func validateHealth(h *store.ServiceHealth) error {
	if h == nil {
		return nil
	}
	for _, p := range []struct {
		name  string
		probe *store.ServiceProbe
	}{{"readiness", h.Readiness}, {"liveness", h.Liveness}} {
		if p.probe == nil {
			continue
		}
		if err := p.probe.ProbeConfig().Validate(); err != nil {
			return fmt.Errorf("health.%s: %w", p.name, err)
		}
	}
	return nil
}

func (rt *Router) handleGetAppHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app health: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, appHealthResource{Name: name, Health: svc.Health})
}

// handleSetAppHealth replaces only an app's health config. A body with
// neither probe (or a null health) clears it.
func (rt *Router) handleSetAppHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req store.ServiceHealth
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateHealth(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	health := &req
	if req.Readiness == nil && req.Liveness == nil && req.ReadyTimeout == 0 {
		health = nil
	}
	rt.saveAppHealth(w, r, name, health)
}

func (rt *Router) handleClearAppHealth(w http.ResponseWriter, r *http.Request) {
	rt.saveAppHealth(w, r, r.PathValue("name"), nil)
}

func (rt *Router) saveAppHealth(w http.ResponseWriter, r *http.Request, name string, health *store.ServiceHealth) {
	err := rt.apps.UpdateServiceHealth(r.Context(), name, health)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set app health failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, appHealthResource{Name: name, Health: health})
}
