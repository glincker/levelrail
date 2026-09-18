package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Route53DNSSecrets is the surface the Route53 DNS-01 settings handlers
// need from internal/secrets.Manager, the same shape CloudflareDNSSecrets
// already establishes for a distinct credential shape: an AWS IAM access
// key pair instead of a single API token.
type Route53DNSSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// route53DNSResource is the wire shape for GET, PUT, and DELETE
// /api/v1/settings/route53-dns. Neither credential half ever appears
// here in either direction, the same convention cloudflareDNSResource
// establishes for its own token.
type route53DNSResource struct {
	Enabled            bool   `json:"enabled"`
	Region             string `json:"region,omitempty"`
	HostedZoneID       string `json:"hosted_zone_id,omitempty"`
	HasAccessKeyID     bool   `json:"has_access_key_id"`
	HasSecretAccessKey bool   `json:"has_secret_access_key"`
}

func (rt *Router) toRoute53DNSResource(ctx context.Context, s store.Route53DNSSettings) route53DNSResource {
	res := route53DNSResource{Enabled: s.Enabled, Region: s.Region, HostedZoneID: s.HostedZoneID}
	if rt.route53DNSSecrets != nil {
		key := store.Route53DNSSecretsKey()
		hasAccessKeyID, err := rt.route53DNSSecrets.Exists(ctx, key, store.Route53DNSAccessKeyIDEnvKey)
		if err != nil {
			rt.logger.Warn("api: check route53 dns access key id failed", slog.String("error", err.Error()))
		}
		res.HasAccessKeyID = hasAccessKeyID
		hasSecretAccessKey, err := rt.route53DNSSecrets.Exists(ctx, key, store.Route53DNSSecretAccessKeyEnvKey)
		if err != nil {
			rt.logger.Warn("api: check route53 dns secret access key failed", slog.String("error", err.Error()))
		}
		res.HasSecretAccessKey = hasSecretAccessKey
	}
	return res
}

// handleGetRoute53DNSSettings handles GET /api/v1/settings/route53-dns.
// AbilityRead, matching GET /api/v1/settings/cloudflare-dns.
func (rt *Router) handleGetRoute53DNSSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.route53DNS.GetRoute53DNSSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get route53 dns settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toRoute53DNSResource(r.Context(), settings))
}

type updateRoute53DNSRequest struct {
	Enabled      bool   `json:"enabled"`
	Region       string `json:"region,omitempty"`
	HostedZoneID string `json:"hosted_zone_id,omitempty"`
	// AccessKeyID and SecretAccessKey are optional on update: empty
	// means "leave the currently stored value unchanged", the same
	// convention updateCloudflareDNSRequest.Token already establishes.
	// The two are set together or not at all: this handler rejects one
	// present without the other, since a partial credential pair can
	// never authenticate.
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

// handleUpdateRoute53DNSSettings handles PUT /api/v1/settings/route53-dns.
// AbilityRoot: instance-level infrastructure config, the same tier PUT
// /api/v1/settings/cloudflare-dns uses. Returns 501 without
// route53DNSSecrets configured (no master key).
func (rt *Router) handleUpdateRoute53DNSSettings(w http.ResponseWriter, r *http.Request) {
	if rt.route53DNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, "route53 dns is not configured on this control plane (no master key set)")
		return
	}

	var req updateRoute53DNSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.AccessKeyID == "") != (req.SecretAccessKey == "") {
		writeError(w, http.StatusBadRequest, "access_key_id and secret_access_key must be set together")
		return
	}

	key := store.Route53DNSSecretsKey()
	hasCreds := req.AccessKeyID != ""
	if !hasCreds {
		existingAccessKeyID, err := rt.route53DNSSecrets.Exists(r.Context(), key, store.Route53DNSAccessKeyIDEnvKey)
		if err != nil {
			rt.logger.Error("api: check route53 dns credentials failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hasCreds = existingAccessKeyID
	}
	if req.Enabled && !hasCreds {
		writeError(w, http.StatusBadRequest, "access_key_id and secret_access_key are required the first time route53 dns is enabled")
		return
	}

	if req.AccessKeyID != "" {
		if err := rt.route53DNSSecrets.SetValue(r.Context(), key, store.Route53DNSAccessKeyIDEnvKey, req.AccessKeyID); err != nil {
			rt.logger.Error("api: save route53 dns access key id failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := rt.route53DNSSecrets.SetValue(r.Context(), key, store.Route53DNSSecretAccessKeyEnvKey, req.SecretAccessKey); err != nil {
			rt.logger.Error("api: save route53 dns secret access key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	settings := store.Route53DNSSettings{Enabled: req.Enabled, Region: req.Region, HostedZoneID: req.HostedZoneID}
	if err := rt.route53DNS.UpdateRoute53DNSSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update route53 dns settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toRoute53DNSResource(r.Context(), settings))
}

// handleDisconnectRoute53DNS handles DELETE /api/v1/settings/route53-dns:
// disables DNS-01 and clears the stored credential pair. Idempotent, the
// same shape handleDisconnectCloudflareDNS establishes.
func (rt *Router) handleDisconnectRoute53DNS(w http.ResponseWriter, r *http.Request) {
	if rt.route53DNSSecrets == nil {
		writeError(w, http.StatusNotImplemented, "route53 dns is not configured on this control plane (no master key set)")
		return
	}

	if err := rt.route53DNSSecrets.DeleteAll(r.Context(), store.Route53DNSSecretsKey()); err != nil {
		rt.logger.Error("api: clear route53 dns credentials failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	settings := store.Route53DNSSettings{Enabled: false}
	if err := rt.route53DNS.UpdateRoute53DNSSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: disconnect route53 dns failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toRoute53DNSResource(r.Context(), settings))
}
