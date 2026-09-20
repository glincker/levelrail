package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// sharedEnvVarResource is the wire shape for one shared env var in the
// combined GET .../env/all view: Value is always "" for a secret entry,
// matching secretKeyResource's own "never echo a value" rule
// (internal/api/secrets.go).
type sharedEnvVarResource struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func sharedEnvVarResources(vars []store.SharedEnvVar) []sharedEnvVarResource {
	out := make([]sharedEnvVarResource, len(vars))
	for i, v := range vars {
		out[i] = sharedEnvVarResource{Key: v.Key, Value: v.Value, Secret: v.Secret}
	}
	return out
}

// sharedEnvSecretScope parameterizes handleListSharedEnvAll/
// handleListSharedEnvSecretKeys/handleSetSharedEnvSecret/
// handleDeleteSharedEnvSecret across the project/organization/
// environment secret-var endpoints, the secret-capable counterpart to
// sharedEnvScope (project_env.go).
type sharedEnvSecretScope struct {
	label       string
	notFoundMsg string
	notFound    error
	load        func(ctx context.Context, id string) error
	listAll     func(ctx context.Context, id string) ([]store.SharedEnvVar, error)
	listKeys    func(ctx context.Context, id string) ([]string, error)
	markSecret  func(ctx context.Context, id, key string) error
	unmark      func(ctx context.Context, id, key string) error
	namespace   func(id string) string
}

func (rt *Router) loadSharedEnvScope(w http.ResponseWriter, r *http.Request, scope sharedEnvSecretScope, id, verb string) bool {
	err := scope.load(r.Context(), id)
	if errors.Is(err, scope.notFound) {
		writeError(w, http.StatusNotFound, scope.notFoundMsg)
		return false
	}
	if err != nil {
		rt.logger.Error("api: "+verb+" "+scope.label+" env: load "+scope.label+" failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	return true
}

// handleListSharedEnvAll handles GET /api/v1/{scope}/{id}/env/all: every
// shared env var, plain and secret-marked alike, one combined list for a
// settings page to render without stitching two separate calls together
// itself.
func (rt *Router) handleListSharedEnvAll(w http.ResponseWriter, r *http.Request, scope sharedEnvSecretScope) {
	id := r.PathValue("id")
	if !rt.loadSharedEnvScope(w, r, scope, id, "list") {
		return
	}

	vars, err := scope.listAll(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: list "+scope.label+" env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sharedEnvVarResources(vars))
}

// handleListSharedEnvSecretKeys handles GET
// /api/v1/{scope}/{id}/env/secrets: every secret-marked shared env var
// key, never a value, mirroring handleListSecrets' own "names only"
// shape for per-app secrets.
func (rt *Router) handleListSharedEnvSecretKeys(w http.ResponseWriter, r *http.Request, scope sharedEnvSecretScope) {
	id := r.PathValue("id")
	if !rt.loadSharedEnvScope(w, r, scope, id, "list") {
		return
	}

	keys, err := scope.listKeys(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: list "+scope.label+" secret env keys failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, keys)
}

type setSharedEnvSecretRequest struct {
	Value string `json:"value"`
}

// handleSetSharedEnvSecret handles PUT
// /api/v1/{scope}/{id}/env/secrets/{key}: encrypts value under
// scope.namespace(id) via Router.secrets (internal/secrets.Manager,
// reused unchanged, not a second encryption path), then marks the key
// secret in the shared-env table. No response body beyond the status,
// matching handleSetSecret's own "never echo a value back" rule.
func (rt *Router) handleSetSharedEnvSecret(w http.ResponseWriter, r *http.Request, scope sharedEnvSecretScope) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}

	id := r.PathValue("id")
	key := r.PathValue("key")
	if !rt.loadSharedEnvScope(w, r, scope, id, "set") {
		return
	}

	var req setSharedEnvSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	// overwriteLocked is always true: shared env var secrets have no
	// per-key lock concept (unlike per-app secrets' SetSecretLock), so
	// there is never a guard here to bypass.
	if err := rt.secrets.SetValueGuarded(r.Context(), scope.namespace(id), key, req.Value, true); err != nil {
		rt.logger.Error("api: set "+scope.label+" secret env var failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := scope.markSecret(r.Context(), id, key); err != nil {
		rt.logger.Error("api: mark "+scope.label+" secret env var failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteSharedEnvSecret handles DELETE
// /api/v1/{scope}/{id}/env/secrets/{key}. Only the shared-env table's
// marker row is removed: the underlying ciphertext in
// service_secret_values is left in place, unreachable once the marker
// is gone, the same orphaned-ciphertext tolerance
// store.DeleteServiceSecrets' own doc comment already accepts.
// Idempotent, so this never needs Router.secrets configured either.
func (rt *Router) handleDeleteSharedEnvSecret(w http.ResponseWriter, r *http.Request, scope sharedEnvSecretScope) {
	id := r.PathValue("id")
	key := r.PathValue("key")
	if !rt.loadSharedEnvScope(w, r, scope, id, "delete") {
		return
	}

	if err := scope.unmark(r.Context(), id, key); err != nil {
		rt.logger.Error("api: delete "+scope.label+" secret env var failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
