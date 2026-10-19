package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/integrity"
)

// IntegrityProbe is the database integrity surface behind the health page and CLI.
type IntegrityProbe interface {
	Status() integrity.Status
	View() (integrity.View, error)
	Check(ctx context.Context, mode string) integrity.Status
	Restore(ctx context.Context, name, by string) (integrity.Pending, error)
	CancelRestore() error
}

type integrityCheckRequest struct {
	Full bool `json:"full"`
}

type integrityRestoreRequest struct {
	Name   string       `json:"name"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

const integrityRestoreStepUpApp = "integrity-restore"

type integrityRestoreResponse struct {
	Pending integrity.Pending     `json:"pending"`
	Restart systemRestartResource `json:"restart"`
}

func (rt *Router) integrityProbe(w http.ResponseWriter) IntegrityProbe {
	p := rt.cpHealth.probe.Load()
	if p == nil || p.Integrity == nil {
		writeError(w, http.StatusNotImplemented, "database integrity checks are not available")
		return nil
	}
	return p.Integrity
}

func (rt *Router) handleGetIntegrity(w http.ResponseWriter, _ *http.Request) {
	ip := rt.integrityProbe(w)
	if ip == nil {
		return
	}
	view, err := ip.View()
	if err != nil {
		rt.internalError(w, "api: integrity view failed", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (rt *Router) handleIntegrityCheck(w http.ResponseWriter, r *http.Request) {
	ip := rt.integrityProbe(w)
	if ip == nil {
		return
	}
	var req integrityCheckRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	mode := integrity.ModeQuick
	if req.Full {
		mode = integrity.ModeFull
	}
	writeJSON(w, http.StatusOK, ip.Check(r.Context(), mode))
}

// handleIntegrityRestore stages a verified snapshot. The next boot applies it before the database opens, so a running connection never sees the file swapped.
func (rt *Router) handleIntegrityRestore(w http.ResponseWriter, r *http.Request) {
	ip := rt.integrityProbe(w)
	if ip == nil {
		return
	}
	var req integrityRestoreRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, integrityRestoreStepUpApp, "restore the control plane database", "database restore"); !ok {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	pending, err := ip.Restore(r.Context(), req.Name, actor)
	switch {
	case errors.Is(err, cpbackup.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rt.logger.Warn("api: database restore staged", slog.String("snapshot", pending.Name), slog.String("actor", actor))
	writeJSON(w, http.StatusAccepted, integrityRestoreResponse{Pending: pending, Restart: rt.restartResource()})
}

func (rt *Router) handleIntegrityCancelRestore(w http.ResponseWriter, r *http.Request) {
	ip := rt.integrityProbe(w)
	if ip == nil {
		return
	}
	if err := ip.CancelRestore(); err != nil {
		rt.internalError(w, "api: cancel staged restore failed", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Info("api: staged database restore cancelled", slog.String("actor", actor))
	w.WriteHeader(http.StatusNoContent)
}
