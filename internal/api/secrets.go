package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type setSecretRequest struct {
	Value string `json:"value"`
}

// handleSetSecret handles PUT /api/v1/apps/{name}/secrets/{key}. It sets
// (or rotates) the encrypted value for one env var of one app. There is
// no response body beyond the status: the value just submitted is
// already known to the caller, and echoing it back or returning any
// derived form of it would be an unnecessary way for a secret to leak
// into logs, browser history, or a proxy's access log further down the
// chain.
//
// This is deliberately not part of appResource / handleUpdateApp: a
// secret's value and an app's desired state have different write paths
// on purpose (one is envelope-encrypted before it ever reaches the
// store, the other is written as-is), and collapsing them into one PUT
// body would make it easy to accidentally round-trip a secret's
// plaintext through GET /api/v1/apps/{name}, exactly what
// store.DesiredService.SecretEnv (names only, no values) exists to
// prevent.
func (rt *Router) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	key := r.PathValue("key")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set secret: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	if err := rt.secrets.SetValue(r.Context(), name, key, req.Value); err != nil {
		rt.logger.Error("api: set secret failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
