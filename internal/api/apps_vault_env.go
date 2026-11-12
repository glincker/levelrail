package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setAppVaultEnvRequest is handleSetAppVaultEnv's request body.
type setAppVaultEnvRequest struct {
	Path string `json:"path"`
	Key  string `json:"key"`
	// Conn (with Path, Key, Version) or Ref (secret://conn/path#key) names a secret connection reference.
	Conn    string `json:"conn,omitempty"`
	Version int    `json:"version,omitempty"`
	Ref     string `json:"ref,omitempty"`
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
	if req.Ref != "" {
		parsed, err := secretsource.ParseRef(req.Ref)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret reference: "+err.Error())
			return
		}
		req = setAppVaultEnvRequest{Conn: parsed.Conn, Path: parsed.Path, Key: parsed.Key, Version: parsed.Version}
	}
	var connRef secretsource.Ref
	if req.Conn != "" {
		parsed, err := envRefOf(appVaultEnvRef{Conn: req.Conn, Path: req.Path, Key: req.Key, Version: req.Version})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret reference: "+err.Error())
			return
		}
		connRef = parsed
		if !rt.allowSecretRefs(w, r, connRef) {
			return
		}
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
		rt.internalError(w, "api: set app vault env: load app failed", err, slog.String("name", name))
		return
	}
	for _, ref := range existing.SecretEnv {
		if ref.Name == key {
			writeError(w, http.StatusBadRequest, "env var \""+key+"\" is already secret-backed, remove it from secret_env first")
			return
		}
	}

	if err := rt.apps.SetServiceVaultEnvVar(r.Context(), name, key, &store.VaultEnvRef{Path: req.Path, Key: req.Key, Conn: req.Conn, Version: req.Version}); err != nil {
		rt.internalError(w, "api: set app vault env failed", err, slog.String("name", name), slog.String("key", key))
		return
	}

	if req.Conn != "" {
		rt.auditSecretRef(r, name, key, connRef.String())
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, appVaultEnvRef{Path: req.Path, Key: req.Key, Conn: req.Conn, Version: req.Version})
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
		rt.internalError(w, "api: clear app vault env failed", err, slog.String("name", name), slog.String("key", key))
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
