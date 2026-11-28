package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/reconcile"
	registryreconcile "github.com/glincker/levelrail/internal/reconcile/registry"
	"github.com/glincker/levelrail/internal/store"
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
	// ProxyRemoteURL is set when the registry runs as a pull-through cache of one upstream.
	ProxyRemoteURL   string `json:"proxy_remote_url,omitempty"`
	ProxyUsername    string `json:"proxy_username,omitempty"`
	HasProxyPassword bool   `json:"has_proxy_password,omitempty"`
	// GC is the unreferenced-layer report, present only when GET asks for it with ?gc=1.
	GC      *registryGCResource `json:"gc,omitempty"`
	Status  string              `json:"status"`
	Message string              `json:"message,omitempty"`
	// Password is set only by handleUpdateRegistrySettings the moment it
	// generates a fresh credential, omitted every other time.
	Password string `json:"password,omitempty"`
}

func (rt *Router) toRegistrySettingsResource(ctx context.Context, s store.RegistrySettings) registrySettingsResource {
	res := registrySettingsResource{Enabled: s.Enabled, Host: s.Host, Username: s.Username,
		ProxyRemoteURL: s.ProxyRemoteURL, ProxyUsername: s.ProxyUsername}

	if rt.registrySecrets != nil {
		exists, err := rt.registrySecrets.Exists(ctx, store.RegistrySettingsSecretsKey(), store.RegistryPasswordEnvKey)
		if err != nil {
			rt.logger.Warn("api: check registry credentials failed", slog.String("error", err.Error()))
		}
		res.HasCredentials = exists
		if s.ProxyRemoteURL != "" {
			hp, perr := rt.registrySecrets.Exists(ctx, store.RegistrySettingsSecretsKey(), store.RegistryProxyPasswordEnvKey)
			if perr != nil {
				rt.logger.Warn("api: check registry proxy password failed", slog.String("error", perr.Error()))
			}
			res.HasProxyPassword = hp
		}
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
		if c.Type != reconcile.ConditionTypeReady {
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
		rt.internalError(w, "api: get registry settings failed", err)
		return
	}
	res := rt.toRegistrySettingsResource(r.Context(), settings)
	if r.URL.Query().Get("gc") == "1" {
		res.GC = rt.registryGCReport(r.Context(), settings.Enabled)
	}
	writeJSON(w, http.StatusOK, res)
}

// updateRegistrySettingsRequest: the proxy fields are pointers so an omitted one keeps its stored value;
// an empty proxy_remote_url turns pull-through mode off.
type updateRegistrySettingsRequest struct {
	Enabled        bool    `json:"enabled"`
	Host           string  `json:"host,omitempty"`
	ProxyRemoteURL *string `json:"proxy_remote_url,omitempty"`
	ProxyUsername  *string `json:"proxy_username,omitempty"`
	ProxyPassword  *string `json:"proxy_password,omitempty"`
}

// validProxyRemote accepts only an http(s) URL with a host, so the upstream cannot be a file or other scheme.
func validProxyRemote(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
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

	current, err := rt.registry.GetRegistrySettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get registry settings failed", err)
		return
	}
	proxyURL, proxyUser := current.ProxyRemoteURL, current.ProxyUsername
	if req.ProxyRemoteURL != nil {
		proxyURL = strings.TrimSpace(*req.ProxyRemoteURL)
		if proxyURL != "" && !validProxyRemote(proxyURL) {
			writeError(w, http.StatusBadRequest, "proxy_remote_url must be an http or https URL")
			return
		}
	}
	if req.ProxyUsername != nil {
		proxyUser = strings.TrimSpace(*req.ProxyUsername)
	}
	if proxyURL == "" {
		proxyUser = ""
	}

	key := store.RegistrySettingsSecretsKey()
	hasCredentials, err := rt.registrySecrets.Exists(r.Context(), key, store.RegistryPasswordEnvKey)
	if err != nil {
		rt.internalError(w, "api: check registry credentials failed", err)
		return
	}

	var generatedPassword string
	if req.Enabled && !hasCredentials {
		password, err := generateRegistryPassword()
		if err != nil {
			rt.internalError(w, "api: generate registry password failed", err)
			return
		}
		if err := rt.registrySecrets.SetValue(r.Context(), key, store.RegistryPasswordEnvKey, password); err != nil {
			rt.internalError(w, "api: save registry password failed", err)
			return
		}
		generatedPassword = password
	}

	if req.ProxyPassword != nil {
		var perr error
		if *req.ProxyPassword == "" || proxyURL == "" {
			if d, ok := rt.registrySecrets.(interface {
				DeleteValue(ctx context.Context, serviceName, envKey string) error
			}); ok {
				perr = d.DeleteValue(r.Context(), key, store.RegistryProxyPasswordEnvKey)
			}
		} else {
			perr = rt.registrySecrets.SetValue(r.Context(), key, store.RegistryProxyPasswordEnvKey, *req.ProxyPassword)
		}
		if perr != nil {
			rt.internalError(w, "api: save registry proxy password failed", perr)
			return
		}
	}

	settings := store.RegistrySettings{Enabled: req.Enabled, Host: req.Host, Username: registryUsername,
		ProxyRemoteURL: proxyURL, ProxyUsername: proxyUser}
	if req.Enabled {
		settings.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := rt.registry.UpdateRegistrySettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: update registry settings failed", err)
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
		rt.internalError(w, "api: clear registry credentials failed", err)
		return
	}
	settings := store.RegistrySettings{}
	if err := rt.registry.UpdateRegistrySettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: disable registry failed", err)
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
