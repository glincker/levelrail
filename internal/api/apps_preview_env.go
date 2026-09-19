package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// setAppPreviewEnvOverrideRequest is handleSetAppPreviewEnvOverride's
// request body.
type setAppPreviewEnvOverrideRequest struct {
	Value string `json:"value"`
}

// handleSetAppPreviewEnvOverride handles
// PUT /api/v1/apps/{name}/preview-env/{key}: declares (or replaces) one
// env var's preview-specific value on an app that already exists. The
// override replaces whatever value that key would otherwise inherit
// (Env, SecretEnv, or VaultEnv) only when a preview environment is next
// created from name (deployPreviewSingle, preview_environments.go),
// never touching name's own running deploy. Mirrors
// handleSetAppVaultEnv's own "dedicated single-field endpoint, not the
// general PUT" shape: an ordinary PUT /api/v1/apps/{name} would silently
// drop this field, see store.DB.SetServicePreviewEnvOverride's own doc
// comment.
func (rt *Router) handleSetAppPreviewEnvOverride(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	var req setAppPreviewEnvOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if key == "" {
		writeError(w, http.StatusBadRequest, "env var key is required")
		return
	}

	if err := rt.apps.SetServicePreviewEnvOverride(r.Context(), name, key, &req.Value); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app preview env override failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": req.Value})
}

// handleClearAppPreviewEnvOverride handles
// DELETE /api/v1/apps/{name}/preview-env/{key}: the reverse of
// handleSetAppPreviewEnvOverride, removing one preview env override.
// Idempotent: clearing a key that was never overridden is not an error,
// the same "disconnect an already-disconnected thing" shape
// handleClearAppVaultEnv already establishes.
func (rt *Router) handleClearAppPreviewEnvOverride(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	if err := rt.apps.SetServicePreviewEnvOverride(r.Context(), name, key, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app preview env override failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
