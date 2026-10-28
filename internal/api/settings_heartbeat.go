package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/heartbeat"
)

// SetHeartbeat enables /api/v1/settings/heartbeat; without it the routes answer 501.
func (rt *Router) SetHeartbeat(s *heartbeat.Service) { rt.heartbeatSvc = s }

type heartbeatUpdateRequest struct {
	URL     *string `json:"url"`
	FailURL *string `json:"fail_url"`
	Body    *bool   `json:"body_enabled"`
}

func (rt *Router) handleGetHeartbeat(w http.ResponseWriter, r *http.Request) {
	if rt.heartbeatSvc == nil {
		writeError(w, http.StatusNotImplemented, "heartbeat is not available on this instance")
		return
	}
	st, err := rt.heartbeatSvc.Status(r.Context())
	if err != nil {
		rt.internalError(w, "api: get heartbeat failed", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handlePutHeartbeat takes the URLs write-only: they are stored as secrets and only their hosts are ever returned.
func (rt *Router) handlePutHeartbeat(w http.ResponseWriter, r *http.Request) {
	if rt.heartbeatSvc == nil {
		writeError(w, http.StatusNotImplemented, "heartbeat is not available on this instance")
		return
	}
	var req heartbeatUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := rt.heartbeatSvc.Apply(r.Context(), heartbeat.Update{URL: req.URL, FailURL: req.FailURL, Body: req.Body})
	switch {
	case errors.Is(err, heartbeat.ErrEnvManaged):
		writeError(w, http.StatusConflict, err.Error()+"; unset it to manage the heartbeat here")
		return
	case errors.Is(err, heartbeat.ErrNoSecrets):
		writeError(w, http.StatusNotImplemented, "storing the heartbeat URL needs a configured master key")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.handleGetHeartbeat(w, r)
}
