package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// branchEnvOverrideResource is the wire shape for one branch-scoped env
// var override. Value is always "" for a secret entry, matching
// sharedEnvVarResource's own "never echo a value" rule.
type branchEnvOverrideResource struct {
	ID            string `json:"id"`
	BranchPattern string `json:"branch_pattern"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Secret        bool   `json:"secret"`
	UpdatedAt     string `json:"updated_at"`
}

func branchEnvOverrideResources(overrides []store.ServiceBranchEnvOverride) []branchEnvOverrideResource {
	out := make([]branchEnvOverrideResource, len(overrides))
	for i, o := range overrides {
		shown, _ := envredact.Value(o.Key, o.Value, nil)
		out[i] = branchEnvOverrideResource{
			ID:            o.ID,
			BranchPattern: o.BranchPattern,
			Key:           o.Key,
			Value:         shown,
			Secret:        o.Secret,
			UpdatedAt:     o.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	return out
}

// handleListAppBranchEnv handles GET /api/v1/apps/{name}/branch-env:
// every branch-scoped override declared for name, across every branch
// pattern, never a secret-marked entry's value.
func (rt *Router) handleListAppBranchEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list app branch env: load app failed", err, slog.String("name", name))
		return
	}

	overrides, err := rt.apps.ListServiceBranchEnvOverrides(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app branch env failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, branchEnvOverrideResources(overrides))
}

// setAppBranchEnvRequest is handleSetAppBranchEnv's request body.
type setAppBranchEnvRequest struct {
	BranchPattern string `json:"branch_pattern"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Secret        bool   `json:"secret"`
}

// Branch-scoped env wins over the app's own env and unscoped preview overrides, and applies only when a preview branch matches BranchPattern.
// Secret values are envelope-encrypted before the row is written, so the row never holds plaintext.
func (rt *Router) handleSetAppBranchEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppBranchEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BranchPattern == "" || req.Key == "" {
		writeError(w, http.StatusBadRequest, "branch_pattern and key are both required")
		return
	}
	if req.Secret && req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required for a secret override")
		return
	}
	if _, err := branchMatchesPattern("x", req.BranchPattern); err != nil {
		writeError(w, http.StatusBadRequest, "branch_pattern is not a valid pattern: "+err.Error())
		return
	}

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app branch env: load app failed", err, slog.String("name", name))
		return
	}

	value := req.Value
	if req.Secret {
		if rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
			return
		}
		namespace := store.BranchEnvOverrideSecretsKey(name, req.BranchPattern)
		if err := rt.secrets.SetValueGuarded(r.Context(), namespace, req.Key, req.Value, true); err != nil {
			rt.internalError(w, "api: set app branch env: encrypt value failed", err, slog.String("name", name), slog.String("key", req.Key))
			return
		}
		value = ""
	}

	id, err := rt.apps.SetServiceBranchEnvOverride(r.Context(), name, req.BranchPattern, req.Key, value, req.Secret)
	if err != nil {
		rt.internalError(w, "api: set app branch env failed", err, slog.String("name", name), slog.String("key", req.Key))
		return
	}

	resource := branchEnvOverrideResource{
		ID:            id,
		BranchPattern: req.BranchPattern,
		Key:           req.Key,
		Secret:        req.Secret,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if !req.Secret {
		resource.Value = req.Value
	}
	writeJSON(w, http.StatusOK, resource)
}

// Deletes by opaque id, since branch names may contain '/'. Only the row is removed; a secret override's ciphertext is left orphaned.
func (rt *Router) handleDeleteAppBranchEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: delete app branch env: load app failed", err, slog.String("name", name))
		return
	}

	if err := rt.apps.DeleteServiceBranchEnvOverride(r.Context(), name, id); errors.Is(err, store.ErrBranchEnvOverrideNotFound) {
		writeError(w, http.StatusNotFound, "branch env override not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: delete app branch env failed", err, slog.String("name", name), slog.String("id", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
