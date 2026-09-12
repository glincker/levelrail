package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	registryreconcile "github.com/GLINCKER/levelrail/internal/reconcile/registry"
	"github.com/GLINCKER/levelrail/internal/store"
)

// registryUsername is the fixed login name generated for the built-in
// registry: unlike an external RegistryCredential (internal/api/
// registry_credentials.go), where the operator already knows and
// supplies a username, this one is platform-generated, so there's no
// operator-meaningful value to derive it from. Fixed, not randomized:
// only the password needs to be secret.
const registryUsername = "levelrail"

// RegistrySecrets is the surface the built-in registry settings handlers
// need from internal/secrets.Manager: set the generated password (first
// enable), check whether one exists, and clear it (disable).
// *secrets.Manager satisfies this structurally, the same "narrow,
// consumer-defined interface" shape CloudflareTunnelSecrets already
// establishes.
type RegistrySecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// registrySettingsResource is the wire shape for GET/PUT/DELETE
// /api/v1/settings/registry. Password is a write-once field: it only
// ever appears in the response to the PUT call that generates it (the
// first time the registry is enabled), never on a later GET or PUT, the
// same "never echo a secret back" boundary registryCredentialResource
// already draws for external credentials, with one necessary exception:
// unlike an operator-supplied external credential, this password is
// platform-generated, so there is no other channel for the operator to
// ever learn it. Losing it means disabling and re-enabling to generate a
// fresh one.
type registrySettingsResource struct {
	Enabled        bool   `json:"enabled"`
	Host           string `json:"host,omitempty"`
	Username       string `json:"username,omitempty"`
	HasCredentials bool   `json:"has_credentials"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	// Password is set only by handleUpdateRegistrySettings the moment it
	// generates a fresh credential, omitted every other time.
	Password string `json:"password,omitempty"`
}

func (rt *Router) toRegistrySettingsResource(ctx context.Context, s store.RegistrySettings) registrySettingsResource {
	res := registrySettingsResource{Enabled: s.Enabled, Host: s.Host, Username: s.Username}

	if rt.registrySecrets != nil {
		exists, err := rt.registrySecrets.Exists(ctx, store.RegistrySettingsSecretsKey(), store.RegistryPasswordEnvKey)
		if err != nil {
			rt.logger.Warn("api: check registry credentials failed", slog.String("error", err.Error()))
		}
		res.HasCredentials = exists
	}

	conditions, err := rt.deploys.GetConditions(ctx, registryreconcile.ControllerName)
	if err != nil {
		rt.logger.Warn("api: get registry conditions failed", slog.String("error", err.Error()))
		res.Status = "stopped"
		return res
	}
	res.Status, res.Message = registryStatus(conditions)
	return res
}

// registryStatus reduces the controller's own Ready condition to the
// three states an operator cares about: "running" (True), "error"
// (False: enabled but not converging), or "stopped" (no conditions
// recorded yet, or Unknown: disabled/never configured), the same
// reduction cloudflareTunnelStatus already performs for its own
// controller.
func registryStatus(conditions []reconcile.Condition) (status, message string) {
	for _, c := range conditions {
		if c.Type != "Ready" {
			continue
		}
		switch c.Status {
		case reconcile.ConditionTrue:
			return "running", ""
		case reconcile.ConditionFalse:
			return "error", c.Message
		default:
			return "stopped", ""
		}
	}
	return "stopped", ""
}

// handleGetRegistrySettings handles GET /api/v1/settings/registry.
// AbilityRead, matching GET /api/v1/settings/cloudflare-tunnel.
func (rt *Router) handleGetRegistrySettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.registry.GetRegistrySettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get registry settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toRegistrySettingsResource(r.Context(), settings))
}

type updateRegistrySettingsRequest struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host,omitempty"`
}

// handleUpdateRegistrySettings handles PUT /api/v1/settings/registry.
// AbilityRoot: instance-level infrastructure config, the same tier PUT
// /api/v1/settings/cloudflare-tunnel uses. Returns 501 without
// registrySecrets configured (no master key). Generates a fresh
// username/password the first time the registry is enabled with no
// credentials yet, returning the password once in this response only;
// every later call (toggling Host, or re-enabling after a disable that
// already cleared credentials would count as "first time" again) leaves
// an existing credential untouched.
func (rt *Router) handleUpdateRegistrySettings(w http.ResponseWriter, r *http.Request) {
	if rt.registrySecrets == nil {
		writeError(w, http.StatusNotImplemented, "the built-in registry is not configured on this control plane (no master key set)")
		return
	}

	var req updateRegistrySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Enabled && req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required to enable the built-in registry")
		return
	}

	key := store.RegistrySettingsSecretsKey()
	hasCredentials, err := rt.registrySecrets.Exists(r.Context(), key, store.RegistryPasswordEnvKey)
	if err != nil {
		rt.logger.Error("api: check registry credentials failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var generatedPassword string
	if req.Enabled && !hasCredentials {
		password, err := generateRegistryPassword()
		if err != nil {
			rt.logger.Error("api: generate registry password failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := rt.registrySecrets.SetValue(r.Context(), key, store.RegistryPasswordEnvKey, password); err != nil {
			rt.logger.Error("api: save registry password failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		generatedPassword = password
	}

	settings := store.RegistrySettings{Enabled: req.Enabled, Host: req.Host, Username: registryUsername}
	if req.Enabled {
		settings.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := rt.registry.UpdateRegistrySettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update registry settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	res := rt.toRegistrySettingsResource(r.Context(), settings)
	res.Password = generatedPassword
	writeJSON(w, http.StatusOK, res)
}

// handleDisableRegistry handles DELETE /api/v1/settings/registry:
// disables the registry and clears its generated credentials, matching
// DELETE /api/v1/settings/cloudflare-tunnel's own "disconnect" tier and
// behavior. Idempotent: disabling an already-disabled registry is not an
// error. A later enable generates a brand new username/password, never
// reuses the cleared one.
func (rt *Router) handleDisableRegistry(w http.ResponseWriter, r *http.Request) {
	if rt.registrySecrets == nil {
		writeError(w, http.StatusNotImplemented, "the built-in registry is not configured on this control plane (no master key set)")
		return
	}

	if err := rt.registrySecrets.DeleteAll(r.Context(), store.RegistrySettingsSecretsKey()); err != nil {
		rt.logger.Error("api: clear registry credentials failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings := store.RegistrySettings{}
	if err := rt.registry.UpdateRegistrySettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: disable registry failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toRegistrySettingsResource(r.Context(), settings))
}

// generateRegistryPassword mirrors backupTargetToken's own shape (9
// random bytes, URL-safe base64, no prefix needed here since this value
// is never displayed alongside other token kinds).
func generateRegistryPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate registry password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
