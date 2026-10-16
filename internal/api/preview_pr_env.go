package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type prPreviewEnvResource struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Secret    bool   `json:"secret"`
	UpdatedAt string `json:"updated_at"`
}

type setPRPreviewEnvRequest struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func prPreviewEnvResources(overrides []store.PRPreviewEnvOverride) []prPreviewEnvResource {
	out := make([]prPreviewEnvResource, len(overrides))
	for i, o := range overrides {
		out[i] = prPreviewEnvResource{ID: o.ID, Key: o.Key, Value: o.Value, Secret: o.Secret, UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339)}
	}
	return out
}

// prNumberFromPath parses the {number} path value, writing the 400 itself.
func prNumberFromPath(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n <= 0 {
		writeError(w, http.StatusBadRequest, "invalid pull request number")
		return 0, false
	}
	return n, true
}

// handleListPRPreviewEnv handles GET /api/v1/apps/{name}/previews/{number}/env.
// A secret override's value is always empty.
func (rt *Router) handleListPRPreviewEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pr, ok := prNumberFromPath(w, r)
	if !ok {
		return
	}
	overrides, err := rt.previewEnvironments.ListPRPreviewEnvOverrides(r.Context(), name, pr)
	if err != nil {
		rt.internalError(w, "api: list pr preview env failed", err, slog.String("name", name), slog.Int("pr_number", pr))
		return
	}
	writeJSON(w, http.StatusOK, prPreviewEnvResources(overrides))
}

// handleSetPRPreviewEnv handles POST /api/v1/apps/{name}/previews/{number}/env.
// The override wins over every other preview env source on that PR's next
// deploy; a secret value is envelope-encrypted and never returned.
func (rt *Router) handleSetPRPreviewEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pr, ok := prNumberFromPath(w, r)
	if !ok {
		return
	}
	var req setPRPreviewEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if req.Secret && req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required for a secret override")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set pr preview env: load app failed", err, slog.String("name", name))
		return
	}

	value := req.Value
	if req.Secret {
		if rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
			return
		}
		if err := rt.secrets.SetValueGuarded(r.Context(), store.PRPreviewEnvSecretsKey(name, pr), req.Key, req.Value, true); err != nil {
			rt.internalError(w, "api: set pr preview env: encrypt value failed", err, slog.String("name", name), slog.String("key", req.Key))
			return
		}
		value = ""
	}
	id, err := rt.previewEnvironments.SetPRPreviewEnvOverride(r.Context(), name, pr, req.Key, value, req.Secret)
	if err != nil {
		rt.internalError(w, "api: set pr preview env failed", err, slog.String("name", name), slog.String("key", req.Key))
		return
	}
	writeJSON(w, http.StatusOK, prPreviewEnvResource{ID: id, Key: req.Key, Value: value, Secret: req.Secret, UpdatedAt: time.Now().UTC().Format(time.RFC3339)})
}

// handleDeletePRPreviewEnv handles DELETE .../previews/{number}/env/{id}.
func (rt *Router) handleDeletePRPreviewEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pr, ok := prNumberFromPath(w, r)
	if !ok {
		return
	}
	err := rt.previewEnvironments.DeletePRPreviewEnvOverride(r.Context(), name, pr, r.PathValue("id"))
	if errors.Is(err, store.ErrPRPreviewEnvOverrideNotFound) {
		writeError(w, http.StatusNotFound, "pull request env override not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete pr preview env failed", err, slog.String("name", name), slog.Int("pr_number", pr))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyPRPreviewEnv sets this pull request's overrides on svc.Env. Errors name
// the key only, never a value.
func (rt *Router) applyPRPreviewEnv(ctx context.Context, svc *spec.Service, appName string, prNumber int) error {
	overrides, err := rt.previewEnvironments.ListPRPreviewEnvOverrides(ctx, appName, prNumber)
	if err != nil {
		return fmt.Errorf("list pr env overrides: %w", err)
	}
	if len(overrides) == 0 {
		return nil
	}
	if svc.Env == nil {
		svc.Env = make(map[string]spec.EnvVar, len(overrides))
	}
	for _, o := range overrides {
		value := o.Value
		if o.Secret {
			if rt.secrets == nil {
				return fmt.Errorf("pr env override %q is a secret but no master key is configured", o.Key)
			}
			if value, err = rt.secrets.Resolve(ctx, store.PRPreviewEnvSecretsKey(appName, prNumber), o.Key); err != nil {
				return fmt.Errorf("resolve pr env override %q: %w", o.Key, err)
			}
		}
		svc.Env[o.Key] = spec.EnvVar{Value: value}
	}
	return nil
}

// clearPRPreviewEnv drops a pull request's overrides and their ciphertext.
// Idempotent, so a retried teardown converges; returns a failure label or "".
func (rt *Router) clearPRPreviewEnv(ctx context.Context, appName string, prNumber int) string {
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, store.PRPreviewEnvSecretsKey(appName, prNumber)); err != nil {
			rt.logger.Error("api: clear pr preview env secrets failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", prNumber))
			return fmt.Sprintf("%s-pr-%d:env-secrets", appName, prNumber)
		}
	}
	if err := rt.previewEnvironments.DeleteAllPRPreviewEnvOverrides(ctx, appName, prNumber); err != nil {
		rt.logger.Error("api: clear pr preview env failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", prNumber))
		return fmt.Sprintf("%s-pr-%d:env-overrides", appName, prNumber)
	}
	return ""
}
