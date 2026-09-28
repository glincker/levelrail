package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type setModelResidencyRequest struct {
	Residency      string `json:"residency"`
	IdleTTLSeconds int    `json:"idle_ttl_seconds"`
}

// handleSetModelResidency handles PUT /api/v1/models/{name}/residency.
func (rt *Router) handleSetModelResidency(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req setModelResidencyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IdleTTLSeconds < 0 {
		writeError(w, http.StatusBadRequest, "idle_ttl_seconds must not be negative")
		return
	}
	if err := rt.models.SetResidency(r.Context(), r.PathValue("name"), req.Residency, time.Duration(req.IdleTTLSeconds)*time.Second); err != nil {
		rt.writeModelError(w, "set model residency", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleWakeModel handles POST /api/v1/models/{name}/wake.
func (rt *Router) handleWakeModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	if err := rt.models.Wake(r.Context(), r.PathValue("name")); err != nil {
		rt.writeModelError(w, "wake model", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleSleepModel handles POST /api/v1/models/{name}/sleep.
func (rt *Router) handleSleepModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	if err := rt.models.Sleep(r.Context(), r.PathValue("name")); err != nil {
		rt.writeModelError(w, "sleep model", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
