package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// oauthProviderSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/oauth[/{provider}]. ClientSecret is never included in
// a response; HasClientSecret only reports whether one is stored.
type oauthProviderSettingsResource struct {
	Provider           string `json:"provider"`
	Enabled            bool   `json:"enabled"`
	ClientID           string `json:"client_id,omitempty"`
	AllowedEmailDomain string `json:"allowed_email_domain,omitempty"`
	HasClientSecret    bool   `json:"has_client_secret"`
}

func (rt *Router) toOAuthProviderSettingsResource(ctx context.Context, s store.OAuthProviderSettings) oauthProviderSettingsResource {
	res := oauthProviderSettingsResource{
		Provider:           s.Provider,
		Enabled:            s.Enabled,
		ClientID:           s.ClientID,
		AllowedEmailDomain: s.AllowedEmailDomain,
	}
	if rt.oauthSecrets == nil {
		return res
	}
	_, err := rt.oauthSecrets.Resolve(ctx, store.OAuthProviderSecretsKey(s.Provider), store.OAuthProviderSecretEnvKey)
	res.HasClientSecret = err == nil
	if err != nil && !errors.Is(err, secrets.ErrValueNotFound) {
		rt.logger.Warn("api: check oauth client secret failed", slog.String("provider", s.Provider), slog.String("error", err.Error()))
	}
	return res
}

// handleListOAuthSettings handles GET /api/v1/settings/oauth: both
// providers' full configuration, AbilityRead like GET /settings/ingress.
func (rt *Router) handleListOAuthSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.oauthSettings.ListOAuthProviderSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: list oauth settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]oauthProviderSettingsResource, 0, len(settings))
	for _, s := range settings {
		out = append(out, rt.toOAuthProviderSettingsResource(r.Context(), s))
	}
	writeJSON(w, http.StatusOK, out)
}

type updateOAuthProviderSettingsRequest struct {
	Enabled            bool   `json:"enabled"`
	ClientID           string `json:"client_id"`
	// ClientSecret is optional on update: empty means "leave the
	// currently stored secret unchanged".
	ClientSecret       string `json:"client_secret,omitempty"`
	AllowedEmailDomain string `json:"allowed_email_domain"`
}

var errOAuthClientSecretRequired = errors.New("client_secret is required the first time a provider is enabled")

// handleUpdateOAuthProviderSettings handles
// PUT /api/v1/settings/oauth/{provider}: AbilityRoot, matching PUT
// /api/v1/settings/ingress, since enabling a provider changes who can
// obtain a session fleet-wide. Returns 501 without OAuthSecrets
// configured (no master key).
func (rt *Router) handleUpdateOAuthProviderSettings(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return
	}
	if rt.oauthSecrets == nil {
		writeError(w, http.StatusNotImplemented, "oauth sign-in is not configured (no master key)")
		return
	}

	var req updateOAuthProviderSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Enabled && req.ClientID == "" {
		writeError(w, http.StatusBadRequest, "client_id is required to enable this provider")
		return
	}

	hasSecret := req.ClientSecret != ""
	if !hasSecret {
		_, err := rt.oauthSecrets.Resolve(r.Context(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey)
		hasSecret = err == nil
	}
	if req.Enabled && !hasSecret {
		writeError(w, http.StatusBadRequest, errOAuthClientSecretRequired.Error())
		return
	}

	if req.ClientSecret != "" {
		if err := rt.oauthSecrets.SetValue(r.Context(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey, req.ClientSecret); err != nil {
			rt.logger.Error("api: save oauth client secret failed", slog.String("provider", provider), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	settings := store.OAuthProviderSettings{
		Provider:           provider,
		Enabled:            req.Enabled,
		ClientID:           req.ClientID,
		AllowedEmailDomain: req.AllowedEmailDomain,
	}
	if err := rt.oauthSettings.UpdateOAuthProviderSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update oauth settings failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toOAuthProviderSettingsResource(r.Context(), settings))
}
