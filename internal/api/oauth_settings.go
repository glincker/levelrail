package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/authengine"
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
	IssuerURL          string `json:"issuer_url,omitempty"`
	DisplayName        string `json:"display_name,omitempty"`
	HasClientSecret    bool   `json:"has_client_secret"`
}

func (rt *Router) toOAuthProviderSettingsResource(ctx context.Context, s store.OAuthProviderSettings) oauthProviderSettingsResource {
	res := oauthProviderSettingsResource{
		Provider:           s.Provider,
		Enabled:            s.Enabled,
		ClientID:           s.ClientID,
		AllowedEmailDomain: s.AllowedEmailDomain,
		IssuerURL:          s.IssuerURL,
		DisplayName:        s.DisplayName,
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
		rt.internalError(w, "api: list oauth settings failed", err)
		return
	}
	out := make([]oauthProviderSettingsResource, 0, len(settings))
	for _, s := range settings {
		out = append(out, rt.toOAuthProviderSettingsResource(r.Context(), s))
	}
	writeJSON(w, http.StatusOK, out)
}

type updateOAuthProviderSettingsRequest struct {
	Enabled  bool   `json:"enabled"`
	ClientID string `json:"client_id"`
	// ClientSecret is optional on update: empty means "leave the
	// currently stored secret unchanged".
	ClientSecret       string `json:"client_secret,omitempty"`
	AllowedEmailDomain string `json:"allowed_email_domain"`
	IssuerURL          string `json:"issuer_url"`
	DisplayName        string `json:"display_name"`
}

var errOAuthClientSecretRequired = errors.New("client_secret is required the first time a provider is enabled")
var errOIDCIssuerURLRequired = errors.New("issuer_url is required to enable the oidc provider")

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
	if req.Enabled && provider == store.OAuthProviderOIDC && req.IssuerURL == "" {
		writeError(w, http.StatusBadRequest, errOIDCIssuerURLRequired.Error())
		return
	}

	if provider == store.OAuthProviderGitLab && req.IssuerURL != "" {
		if u, err := url.Parse(strings.TrimSpace(req.IssuerURL)); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			writeError(w, http.StatusBadRequest, "instance url must be an https URL without credentials, query or fragment")
			return
		}
	}
	if provider == store.OAuthProviderCognito && req.IssuerURL != "" {
		if _, _, err := authengine.ParseCognitoPoolID(req.IssuerURL); err != nil {
			writeError(w, http.StatusBadRequest, "user pool id must look like us-east-1_AbC123")
			return
		}
	}
	if provider != store.OAuthProviderOIDC && provider != store.OAuthProviderGitLab && provider != store.OAuthProviderCognito {
		req.IssuerURL = ""
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
			rt.internalError(w, "api: save oauth client secret failed", err, slog.String("provider", provider))
			return
		}
	}

	settings := store.OAuthProviderSettings{
		Provider:           provider,
		Enabled:            req.Enabled,
		ClientID:           req.ClientID,
		AllowedEmailDomain: req.AllowedEmailDomain,
		IssuerURL:          req.IssuerURL,
		DisplayName:        req.DisplayName,
	}
	if err := rt.oauthSettings.UpdateOAuthProviderSettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: update oauth settings failed", err, slog.String("provider", provider))
		return
	}
	rt.authLibOAuthInvalidate(provider)
	writeJSON(w, http.StatusOK, rt.toOAuthProviderSettingsResource(r.Context(), settings))
}
