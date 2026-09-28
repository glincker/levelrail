package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

// SecretBinder is the surface the secrets binding routes need from
// internal/secrets.Manager: count legacy unbound values and rebind them.
type SecretBinder interface {
	BindingStatus(ctx context.Context) (secrets.BindingStatus, error)
	Rebind(ctx context.Context) (secrets.RebindResult, error)
}

// WithSecretBinding enables GET /api/v1/system/secrets/binding, POST
// /api/v1/system/secrets/rebind, the doctor's secret_binding check, and
// the automatic rebind after a master key rotation.
func WithSecretBinding(b SecretBinder) Option {
	return func(rt *Router) { rt.secretBinder = b }
}

type secretBindingResponse struct {
	Total  int `json:"total"`
	Bound  int `json:"bound"`
	Legacy int `json:"legacy"`
}

type secretRebindFailure struct {
	Owner  string `json:"owner"`
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

type secretRebindResponse struct {
	Scanned      int                   `json:"scanned"`
	Rebound      int                   `json:"rebound"`
	AlreadyBound int                   `json:"alreadyBound"`
	Changed      int                   `json:"changed"`
	FailedCount  int                   `json:"failedCount"`
	Failed       []secretRebindFailure `json:"failed"`
	Remaining    int                   `json:"remaining"`
}

func toSecretRebindResponse(r secrets.RebindResult) secretRebindResponse {
	failed := make([]secretRebindFailure, 0, len(r.Failed))
	for _, f := range r.Failed {
		failed = append(failed, secretRebindFailure(f))
	}
	return secretRebindResponse{
		Scanned: r.Scanned, Rebound: r.Rebound, AlreadyBound: r.AlreadyBound, Changed: r.Changed,
		FailedCount: r.FailedCount, Failed: failed, Remaining: r.Remaining,
	}
}

func (rt *Router) handleGetSecretBinding(w http.ResponseWriter, r *http.Request) {
	if rt.secretBinder == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key loaded)")
		return
	}
	status, err := rt.secretBinder.BindingStatus(r.Context())
	if err != nil {
		rt.logger.Error("api: secret binding status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not read secret binding status")
		return
	}
	writeJSON(w, http.StatusOK, secretBindingResponse(status))
}

// handleRebindSecrets re-encrypts every legacy unbound secret value with
// its slot binding. Safe to repeat: already-bound values are skipped.
func (rt *Router) handleRebindSecrets(w http.ResponseWriter, r *http.Request) {
	if rt.secretBinder == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key loaded)")
		return
	}
	result, err := rt.secretBinder.Rebind(r.Context())
	if err != nil {
		rt.logger.Error("api: rebind secrets failed", slog.String("error", err.Error()), slog.Int("rebound", result.Rebound))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("rebind stopped after %d values, progress is kept and a rerun resumes: %s", result.Rebound, err))
		return
	}
	writeJSON(w, http.StatusOK, toSecretRebindResponse(result))
}

// rebindAfterRotation runs a rebind once a rotation has committed. The
// rotation already succeeded, so a failure here is only a warning.
func (rt *Router) rebindAfterRotation(ctx context.Context) (*secretRebindResponse, string) {
	if rt.secretBinder == nil {
		return nil, ""
	}
	result, err := rt.secretBinder.Rebind(ctx)
	if err != nil {
		rt.logger.Error("api: rebind after master key rotation failed", slog.String("error", err.Error()))
		return nil, "the master key was rotated, but binding legacy secret values to their slots stopped early: rerun it with secrets rebind"
	}
	resp := toSecretRebindResponse(result)
	return &resp, ""
}

func (rt *Router) doctorCheckSecretBinding(ctx context.Context) doctorCheckResource {
	const code, name = "secret_binding", "Secret slot binding"
	if rt.secretBinder == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no master key configured"}
	}
	status, err := rt.secretBinder.BindingStatus(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not count secret formats: %s", err)}
	}
	if status.Legacy > 0 {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d of %d secret values predate slot binding", status.Legacy, status.Total),
			Fix:      rt.cliName() + " secrets rebind",
			DocsPath: "/master-key-rotation#binding-secrets-to-their-slot",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("all %d secret values are bound to their slot", status.Total)}
}
