package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// lookupAppAttempt loads the app and one of its deploy attempts, writing the
// error response itself when either is missing.
func (rt *Router) lookupAppAttempt(w http.ResponseWriter, r *http.Request) (*store.DeployAttempt, bool) {
	name, deployID := r.PathValue("name"), r.PathValue("deployId")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return nil, false
		}
		rt.internalError(w, "api: deploy action: load app failed", err, slog.String("name", name))
		return nil, false
	}
	a, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) || (err == nil && a.ServiceName != name) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: deploy action: load attempt failed", err, slog.String("deploy_id", deployID))
		return nil, false
	}
	return a, true
}

// handleCancelDeploy handles POST /api/v1/apps/{name}/deploys/{deployId}/cancel.
// A queued or held deploy is dropped; a running one has its build stopped
// before it can write desired state, so the serving release is never touched.
// A deploy that already cut traffic answers 409.
func (rt *Router) handleCancelDeploy(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy cancellation is not configured on this control plane")
		return
	}
	a, ok := rt.lookupAppAttempt(w, r)
	if !ok {
		return
	}
	by := rt.eventActor(r)
	ctx := r.Context()

	switch a.Status {
	case store.DeployAttemptStatusQueued, store.DeployAttemptStatusHeld:
		moved, err := rt.deploySafety.CancelDeployAttempt(ctx, a.ID, a.Status, by, time.Now())
		if err != nil {
			rt.internalError(w, "api: cancel deploy failed", err, slog.String("attempt_id", a.ID))
			return
		}
		if !moved {
			rt.writeCancelConflict(w, r, a.ID, "the deploy started or finished before the cancel landed")
			return
		}
	case store.DeployAttemptStatusRunning:
		switch rt.cancels.Cancel(a.ID, by) {
		case deploy.CancelTooLate:
			writeError(w, http.StatusConflict, "this deploy already cut over to the new release (desired state written), so it can no longer be canceled; roll back instead")
			return
		case deploy.CancelUnknown:
			rt.writeCancelConflict(w, r, a.ID, "this deploy is not in progress on this control plane")
			return
		}
		if _, err := rt.deploySafety.CancelDeployAttempt(ctx, a.ID, store.DeployAttemptStatusRunning, by, time.Now()); err != nil {
			rt.internalError(w, "api: record canceled deploy failed", err, slog.String("attempt_id", a.ID))
			return
		}
	default:
		writeError(w, http.StatusConflict, "deploy already "+a.Status+", nothing to cancel")
		return
	}

	rt.logger.Info("api: deploy canceled", slog.String("attempt_id", a.ID), slog.String("name", a.ServiceName), slog.String("by", by))
	rt.DrainDeployQueue(ctx)
	rt.writeAttempt(w, r, a.ID)
}

func (rt *Router) writeCancelConflict(w http.ResponseWriter, r *http.Request, id, why string) {
	cur, err := rt.deployAttempts.GetDeployAttempt(r.Context(), id)
	if err != nil {
		rt.logger.Warn("api: cancel conflict: reload attempt failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		why += " (current state unknown)"
	} else {
		why += " (now " + cur.Status + ")"
	}
	writeError(w, http.StatusConflict, why)
}

func (rt *Router) writeAttempt(w http.ResponseWriter, r *http.Request, id string) {
	a, err := rt.deployAttempts.GetDeployAttempt(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: reload deploy attempt failed", err, slog.String("attempt_id", id))
		return
	}
	res := toDeployAttemptResource(*a)
	if attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), a.ServiceName); err == nil {
		rt.applyWait(r.Context(), &res, *a, attempts)
		applyDeployOutcome(&res, *a, len(attempts) > 0 && attempts[0].ID == a.ID, rt.rolloutConditions(r.Context(), a.ServiceName))
	}
	writeJSON(w, http.StatusOK, res)
}

type cancelSupersededResource struct {
	Enabled bool `json:"enabled"`
}

// handleGetCancelSuperseded handles GET /api/v1/apps/{name}/cancel-superseded.
func (rt *Router) handleGetCancelSuperseded(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy queue is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	on, err := rt.deploySafety.GetServiceCancelSuperseded(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get cancel-superseded failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, cancelSupersededResource{Enabled: on})
}

// handleSetCancelSuperseded handles PUT /api/v1/apps/{name}/cancel-superseded.
// On, a newer queued deploy of a branch supersedes older queued ones of the
// same branch. Deploys that already started building are never auto-canceled.
func (rt *Router) handleSetCancelSuperseded(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy queue is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	var req cancelSupersededResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := rt.deploySafety.SetServiceCancelSuperseded(r.Context(), name, req.Enabled); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: set cancel-superseded failed", err, slog.String("name", name))
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: "Cancel superseded deploys " + onOff(req.Enabled)})
	writeJSON(w, http.StatusOK, req)
}

func onOff(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
