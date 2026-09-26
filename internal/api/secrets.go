package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type setSecretRequest struct {
	Value           string `json:"value"`
	OverwriteLocked bool   `json:"overwrite_locked"`
}

// handleSetSecret handles PUT /api/v1/apps/{name}/secrets/{key}. It sets
// (or rotates) the encrypted value for one env var of one app. There is
// no response body beyond the status: the value just submitted is
// already known to the caller, and echoing it back or returning any
// derived form of it would be an unnecessary way for a secret to leak
// into logs, browser history, or a proxy's access log further down the
// chain.
//
// If the key already has a value and is locked, this returns 409 unless
// the request body sets overwrite_locked: a reversible guard against an
// accidental paste/save clobbering a production credential, not a
// permanent write-once property.
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

	err = rt.secrets.SetValueGuarded(r.Context(), name, key, req.Value, req.OverwriteLocked)
	if errors.Is(err, secrets.ErrSecretLocked) {
		writeError(w, http.StatusConflict, "this key is locked, set overwrite_locked to confirm replacing it")
		return
	}
	if err != nil {
		rt.logger.Error("api: set secret failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.declareSecretEnv(r.Context(), name, key, true)
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventSecretChange, Keys: []string{key}, Title: "Secret set: " + key})
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// secretEnvDeclarer and secretValueDeleter are the narrow store writes behind
// declaring a key as secret-backed and deleting its value. *store.DB
// provides both.
type secretEnvDeclarer interface {
	SetServiceSecretEnvDeclared(ctx context.Context, name, key string, declared bool) (bool, error)
}

type secretValueDeleter interface {
	DeleteSecretValue(ctx context.Context, serviceName, envKey string) (bool, error)
}

// declareSecretEnv makes key a secret-backed env name on the app, so its
// stored value is injected at container creation. Without it a stored value
// is never read. Failure is logged: the value itself is already saved.
func (rt *Router) declareSecretEnv(ctx context.Context, name, key string, declared bool) bool {
	d, ok := rt.apps.(secretEnvDeclarer)
	if !ok {
		return false
	}
	changed, err := d.SetServiceSecretEnvDeclared(ctx, name, key, declared)
	if err != nil {
		rt.logger.Error("api: declare secret env failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		return false
	}
	return changed
}

// handleDeleteSecret handles DELETE /api/v1/apps/{name}/secrets/{key}: removes
// the stored value and undeclares the key. A locked secret needs force=true.
func (rt *Router) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	deleter, ok := rt.apps.(secretValueDeleter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "deleting secrets is not supported by this store")
		return
	}
	name, key := r.PathValue("name"), r.PathValue("key")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete secret: load app failed", err, slog.String("name", name))
		return
	}
	keys, err := rt.secrets.ListKeys(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: delete secret: list keys failed", err, slog.String("name", name))
		return
	}
	for _, k := range keys {
		if k.Key == key && k.Locked && r.URL.Query().Get("force") != "true" {
			writeError(w, http.StatusConflict, "this key is locked, pass force=true to delete it")
			return
		}
	}
	existed, err := deleter.DeleteSecretValue(r.Context(), name, key)
	if err != nil {
		rt.internalError(w, "api: delete secret failed", err, slog.String("name", name), slog.String("key", key))
		return
	}
	undeclared := rt.declareSecretEnv(r.Context(), name, key, false)
	if !existed && !undeclared && !slices.Contains(store.SecretEnvNames(svc.SecretEnv), key) {
		writeError(w, http.StatusNotFound, "no secret with this key")
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventSecretChange, Keys: []string{key}, Title: "Secret deleted: " + key})
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

type secretKeyResource struct {
	Key    string `json:"key"`
	Locked bool   `json:"locked"`
	// UpdatedAt is when this key's value was last set or rotated
	// (RFC3339), and Stale reports whether that's older than the
	// effective secret rotation warning threshold
	// (Router.effectiveSecretRotationWarnAge, secret_rotation.go),
	// letting a UI show a secret's age and flag it as due for rotation.
	UpdatedAt string `json:"updated_at"`
	Stale     bool   `json:"stale"`
}

// handleListSecrets handles GET /api/v1/apps/{name}/secrets: every known
// secret key for the app, with its locked state, never a value. This is
// the "names only" companion GET the PUT-only secrets route has always
// been missing (store.DesiredService.SecretEnv holds names from the app
// spec's { secret: true } declarations, but this is names of keys that
// actually have a stored value, a different and more useful set for a
// developer-view UI to render).
func (rt *Router) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list secrets: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	keys, err := rt.secrets.ListKeys(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list secrets failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]secretKeyResource, len(keys))
	for i, k := range keys {
		out[i] = secretKeyResource{
			Key:       k.Key,
			Locked:    k.Locked,
			UpdatedAt: k.UpdatedAt.UTC().Format(time.RFC3339),
			Stale:     rt.secretIsStale(k.UpdatedAt),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type setSecretLockRequest struct {
	Locked bool `json:"locked"`
}

// handleSetSecretLock handles POST /api/v1/apps/{name}/secrets/{key}/lock.
// Reversible either direction: this is a safety guard against accidental
// overwrite, not a permanent write-once marker.
func (rt *Router) handleSetSecretLock(w http.ResponseWriter, r *http.Request) {
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
		rt.logger.Error("api: set secret lock: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setSecretLockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err = rt.secrets.SetLocked(r.Context(), name, key, req.Locked)
	if errors.Is(err, store.ErrSecretValueNotFound) {
		writeError(w, http.StatusNotFound, "no secret value set for this key yet")
		return
	}
	if err != nil {
		rt.logger.Error("api: set secret lock failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
