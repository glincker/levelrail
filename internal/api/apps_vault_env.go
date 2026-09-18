package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// setAppVaultEnvRequest is handleSetAppVaultEnv's request body.
type setAppVaultEnvRequest struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

// handleSetAppVaultEnv handles PUT /api/v1/apps/{name}/vault-env/{key}:
// declares (or replaces) one env var as resolving live from the
// platform's configured external Vault instance, the UI/CLI-facing
// equivalent of app.yaml's own { vault: { path, key } } env var syntax
// for an app that already exists, mirroring handleSetAppDatabase's own
// "dedicated single-field endpoint, not the general PUT" shape: an
// ordinary PUT /api/v1/apps/{name} would silently drop this field (and
// every other one it doesn't itself carry forward), see
// store.DB.SetServiceVaultEnvVar's own doc comment.
//
// No value is stored here, only the reference: unlike
// PUT /api/v1/apps/{name}/secrets/{key}, there is nothing to encrypt.
func (rt *Router) handleSetAppVaultEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	var req setAppVaultEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" || req.Key == "" {
		writeError(w, http.StatusBadRequest, "path and key are both required")
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set app vault env: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, secretKey := range existing.SecretEnv {
		if secretKey == key {
			writeError(w, http.StatusBadRequest, "env var \""+key+"\" is already secret-backed, remove it from secret_env first")
			return
		}
	}

	if err := rt.apps.SetServiceVaultEnvVar(r.Context(), name, key, &store.VaultEnvRef{Path: req.Path, Key: req.Key}); err != nil {
		rt.logger.Error("api: set app vault env failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, appVaultEnvRef(req))
}

// handleClearAppVaultEnv handles DELETE /api/v1/apps/{name}/vault-env/{key}:
// the reverse of handleSetAppVaultEnv, removing one Vault-sourced env var
// declaration. Idempotent: clearing a key that was never set is not an
// error, the same "disconnect an already-disconnected thing" shape
// handleDisconnectCloudflareTunnel already establishes.
func (rt *Router) handleClearAppVaultEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	if err := rt.apps.SetServiceVaultEnvVar(r.Context(), name, key, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app vault env failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
