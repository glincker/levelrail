package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/cloudflaretunnel"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CloudflareTunnelSecrets is the surface the Cloudflare Tunnel settings
// handlers need from internal/secrets.Manager: set the token (PUT),
// check whether one exists (GET's has_token, and PUT's "was a token
// already set" check for the omitted-token case), and clear it
// (DELETE). *secrets.Manager satisfies this structurally, the same
// "narrow, consumer-defined interface" shape GitHubAppSecrets already
// establishes.
type CloudflareTunnelSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// cloudflareTunnelResource is the wire shape for GET, PUT, and DELETE
// /api/v1/settings/cloudflare-tunnel. The token itself never appears
// here in either direction: PUT accepts one as a write-only request
// field (see updateCloudflareTunnelRequest), GET/PUT/DELETE responses
// only ever report HasToken.
type cloudflareTunnelResource struct {
	Enabled  bool `json:"enabled"`
	HasToken bool `json:"has_token"`
	// Status summarizes the cloudflared container's actual observed
	// state, derived fresh from reconcile_status
	// (cloudflaretunnel.ControllerName's own conditions) on every read,
	// the same "status lives in reconcile conditions, not a settings
	// column" convention handleDatabaseStatus already follows: "connected"
	// (the container is up), "error" (enabled but not converging, e.g. no
	// token yet or a create/start failure), or "disconnected" (disabled,
	// or never configured).
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (rt *Router) toCloudflareTunnelResource(ctx context.Context, s store.CloudflareTunnelSettings) cloudflareTunnelResource {
	res := cloudflareTunnelResource{Enabled: s.Enabled}

	if rt.cloudflareTunnelSecrets != nil {
		exists, err := rt.cloudflareTunnelSecrets.Exists(ctx, store.CloudflareTunnelSecretsKey(), store.CloudflareTunnelTokenEnvKey)
		if err != nil {
			rt.logger.Warn("api: check cloudflare tunnel token failed", slog.String("error", err.Error()))
		}
		res.HasToken = exists
	}

	conditions, err := rt.deploys.GetConditions(ctx, cloudflaretunnel.ControllerName)
	if err != nil {
		rt.logger.Warn("api: get cloudflare tunnel conditions failed", slog.String("error", err.Error()))
		res.Status = "disconnected"
		return res
	}
	res.Status, res.Message = cloudflareTunnelStatus(conditions)
	return res
}

// cloudflareTunnelStatus reduces the controller's own Ready condition to
// the three states an operator cares about: "connected" (True),
// "error" (False: enabled but not converging), or "disconnected" (no
// conditions recorded yet, or Unknown: disabled/never configured).
func cloudflareTunnelStatus(conditions []reconcile.Condition) (status, message string) {
	for _, c := range conditions {
		if c.Type != "Ready" {
			continue
		}
		switch c.Status {
		case reconcile.ConditionTrue:
			return "connected", ""
		case reconcile.ConditionFalse:
			return "error", c.Message
		default:
			return "disconnected", ""
		}
	}
	return "disconnected", ""
}

// handleGetCloudflareTunnelSettings handles GET
// /api/v1/settings/cloudflare-tunnel. AbilityRead, matching GET
// /api/v1/settings/email and GET /api/v1/settings/ingress.
func (rt *Router) handleGetCloudflareTunnelSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.cloudflareTunnel.GetCloudflareTunnelSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get cloudflare tunnel settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareTunnelResource(r.Context(), settings))
}

type updateCloudflareTunnelRequest struct {
	Enabled bool `json:"enabled"`
	// Token is optional on update: empty means "leave the currently
	// stored token unchanged", the same convention
	// updateOAuthProviderSettingsRequest.ClientSecret already establishes.
	Token string `json:"token,omitempty"`
}

// handleUpdateCloudflareTunnelSettings handles PUT
// /api/v1/settings/cloudflare-tunnel. AbilityRoot: instance-level
// infrastructure config, the same tier PUT /api/v1/settings/oauth/{provider}
// and PUT /api/v1/settings/ingress use. Returns 501 without
// cloudflareTunnelSecrets configured (no master key).
func (rt *Router) handleUpdateCloudflareTunnelSettings(w http.ResponseWriter, r *http.Request) {
	if rt.cloudflareTunnelSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloudflare tunnel is not configured on this control plane (no master key set)")
		return
	}

	var req updateCloudflareTunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key := store.CloudflareTunnelSecretsKey()
	hasToken := req.Token != ""
	if !hasToken {
		exists, err := rt.cloudflareTunnelSecrets.Exists(r.Context(), key, store.CloudflareTunnelTokenEnvKey)
		if err != nil {
			rt.logger.Error("api: check cloudflare tunnel token failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hasToken = exists
	}
	if req.Enabled && !hasToken {
		writeError(w, http.StatusBadRequest, "token is required the first time cloudflare tunnel is enabled")
		return
	}

	if req.Token != "" {
		if err := rt.cloudflareTunnelSecrets.SetValue(r.Context(), key, store.CloudflareTunnelTokenEnvKey, req.Token); err != nil {
			rt.logger.Error("api: save cloudflare tunnel token failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	settings := store.CloudflareTunnelSettings{Enabled: req.Enabled}
	if err := rt.cloudflareTunnel.UpdateCloudflareTunnelSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update cloudflare tunnel settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareTunnelResource(r.Context(), settings))
}

// handleDisconnectCloudflareTunnel handles DELETE
// /api/v1/settings/cloudflare-tunnel: disables the tunnel and clears the
// stored token, matching DELETE /api/v1/github-app's own AbilityRoot
// tier for a comparable "disconnect an integration" action. Idempotent:
// disconnecting an already-disconnected tunnel is not an error.
func (rt *Router) handleDisconnectCloudflareTunnel(w http.ResponseWriter, r *http.Request) {
	if rt.cloudflareTunnelSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloudflare tunnel is not configured on this control plane (no master key set)")
		return
	}

	if err := rt.cloudflareTunnelSecrets.DeleteAll(r.Context(), store.CloudflareTunnelSecretsKey()); err != nil {
		rt.logger.Error("api: clear cloudflare tunnel token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings := store.CloudflareTunnelSettings{Enabled: false}
	if err := rt.cloudflareTunnel.UpdateCloudflareTunnelSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: disconnect cloudflare tunnel failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareTunnelResource(r.Context(), settings))
}
