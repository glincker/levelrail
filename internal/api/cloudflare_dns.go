package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CloudflareDNSSecrets is the surface the Cloudflare DNS-01 settings
// handlers need from internal/secrets.Manager, the same shape
// CloudflareTunnelSecrets already establishes for a distinct credential
// (a scoped Cloudflare API token, not the cloudflared connector token).
type CloudflareDNSSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// cloudflareDNSResource is the wire shape for GET, PUT, and DELETE
// /api/v1/settings/cloudflare-dns. The token itself never appears here
// in either direction, the same convention cloudflareTunnelResource
// establishes.
type cloudflareDNSResource struct {
	Enabled  bool `json:"enabled"`
	HasToken bool `json:"has_token"`
}

func (rt *Router) toCloudflareDNSResource(ctx context.Context, s store.CloudflareDNSSettings) cloudflareDNSResource {
	res := cloudflareDNSResource{Enabled: s.Enabled}
	if rt.cloudflareDNSSecrets != nil {
		exists, err := rt.cloudflareDNSSecrets.Exists(ctx, store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey)
		if err != nil {
			rt.logger.Warn("api: check cloudflare dns token failed", slog.String("error", err.Error()))
		}
		res.HasToken = exists
	}
	return res
}

// handleGetCloudflareDNSSettings handles GET
// /api/v1/settings/cloudflare-dns. AbilityRead, matching GET
// /api/v1/settings/cloudflare-tunnel.
func (rt *Router) handleGetCloudflareDNSSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.cloudflareDNS.GetCloudflareDNSSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get cloudflare dns settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareDNSResource(r.Context(), settings))
}

type updateCloudflareDNSRequest struct {
	Enabled bool `json:"enabled"`
	// Token is optional on update: empty means "leave the currently
	// stored token unchanged", the same convention
	// updateCloudflareTunnelRequest.Token already establishes.
	Token string `json:"token,omitempty"`
}

// handleUpdateCloudflareDNSSettings handles PUT
// /api/v1/settings/cloudflare-dns. AbilityRoot: instance-level
// infrastructure config, the same tier PUT /api/v1/settings/cloudflare-tunnel
// uses. Returns 501 without cloudflareDNSSecrets configured (no master
// key).
func (rt *Router) handleUpdateCloudflareDNSSettings(w http.ResponseWriter, r *http.Request) {
	if rt.cloudflareDNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloudflare dns is not configured on this control plane (no master key set)")
		return
	}

	var req updateCloudflareDNSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key := store.CloudflareDNSSecretsKey()
	hasToken := req.Token != ""
	if !hasToken {
		exists, err := rt.cloudflareDNSSecrets.Exists(r.Context(), key, store.CloudflareDNSTokenEnvKey)
		if err != nil {
			rt.internalError(w, "api: check cloudflare dns token failed", err)
			return
		}
		hasToken = exists
	}
	if req.Enabled && !hasToken {
		writeError(w, http.StatusBadRequest, "token is required the first time cloudflare dns is enabled")
		return
	}

	if req.Token != "" {
		if err := rt.cloudflareDNSSecrets.SetValue(r.Context(), key, store.CloudflareDNSTokenEnvKey, req.Token); err != nil {
			rt.internalError(w, "api: save cloudflare dns token failed", err)
			return
		}
	}

	settings := store.CloudflareDNSSettings{Enabled: req.Enabled}
	if err := rt.cloudflareDNS.UpdateCloudflareDNSSettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: update cloudflare dns settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareDNSResource(r.Context(), settings))
}

// handleDisconnectCloudflareDNS handles DELETE
// /api/v1/settings/cloudflare-dns: disables DNS-01 and clears the stored
// token. Idempotent, the same shape handleDisconnectCloudflareTunnel
// establishes.
func (rt *Router) handleDisconnectCloudflareDNS(w http.ResponseWriter, r *http.Request) {
	if rt.cloudflareDNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloudflare dns is not configured on this control plane (no master key set)")
		return
	}

	if err := rt.cloudflareDNSSecrets.DeleteAll(r.Context(), store.CloudflareDNSSecretsKey()); err != nil {
		rt.internalError(w, "api: clear cloudflare dns token failed", err)
		return
	}
	settings := store.CloudflareDNSSettings{Enabled: false}
	if err := rt.cloudflareDNS.UpdateCloudflareDNSSettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: disconnect cloudflare dns failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudflareDNSResource(r.Context(), settings))
}
