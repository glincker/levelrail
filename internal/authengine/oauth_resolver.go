package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
	githubprov "github.com/glincker/theauth-go/v2/provider/github"
	googleprov "github.com/glincker/theauth-go/v2/provider/google"
	microsoftprov "github.com/glincker/theauth-go/v2/provider/microsoft"
	oidcprov "github.com/glincker/theauth-go/v2/provider/oidc"

	"github.com/GLINCKER/levelrail/internal/store"
)

const oidcDiscoveryTimeout = 10 * time.Second

// Scope sets match what the in-house flow requests, so consent screens do not change.
var (
	googleScopes    = []string{"openid", "email", "profile"}
	githubScopes    = []string{"read:user", "user:email"}
	microsoftScopes = []string{"openid", "email", "profile", "https://graph.microsoft.com/User.Read"}
	oidcScopes      = []string{"openid", "email", "profile"}
)

// providerResolver builds library providers from the dashboard settings on
// every cache miss, so edits apply without a restart.
type providerResolver struct{ rt *oauthRuntime }

func newProviderResolver(rt *oauthRuntime) *providerResolver { return &providerResolver{rt: rt} }

func validOAuthProvider(name string) bool {
	switch name {
	case store.OAuthProviderGoogle, store.OAuthProviderGitHub, store.OAuthProviderMicrosoft, store.OAuthProviderOIDC:
		return true
	}
	return false
}

// Resolve returns the enabled provider for name. A settings or secret read
// failure is an error, never "not found", so the flow fails closed.
func (r *providerResolver) Resolve(ctx context.Context, name string) (theauth.Provider, bool, error) {
	if !validOAuthProvider(name) {
		return nil, false, nil
	}
	w := r.rt.wiring
	s, err := w.Settings.GetOAuthProviderSettings(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("authengine: load oauth settings for %s: %w", name, err)
	}
	if !s.Enabled {
		return nil, false, nil
	}
	secret, err := w.Secrets.Resolve(ctx, store.OAuthProviderSecretsKey(name), store.OAuthProviderSecretEnvKey)
	if err != nil {
		return nil, false, fmt.Errorf("authengine: resolve client secret for %s: %w", name, err)
	}
	if s.ClientID == "" || secret == "" {
		return nil, false, fmt.Errorf("authengine: provider %s is enabled without client credentials", name)
	}
	inner, err := r.build(ctx, s, secret)
	if err != nil {
		return nil, false, err
	}
	return r.rt.gate(inner, s), true, nil
}

func (r *providerResolver) httpClient() *http.Client {
	if r.rt.wiring.HTTP != nil {
		return r.rt.wiring.HTTP
	}
	return &http.Client{Timeout: oidcDiscoveryTimeout}
}

func (r *providerResolver) build(ctx context.Context, s store.OAuthProviderSettings, secret string) (theauth.Provider, error) {
	ep := r.rt.wiring.Endpoints
	hc := r.httpClient()
	switch s.Provider {
	case store.OAuthProviderGoogle:
		return googleprov.New(googleprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: googleScopes, HTTPClient: hc,
			AuthorizeURL: ep.GoogleAuthorize, TokenURL: ep.GoogleToken, UserInfoURL: ep.GoogleUserInfo,
		}), nil
	case store.OAuthProviderGitHub:
		return githubprov.New(githubprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: githubScopes, HTTPClient: hc,
			AuthorizeURL: ep.GitHubAuthorize, TokenURL: ep.GitHubToken, UserURL: ep.GitHubUser, EmailsURL: ep.GitHubEmails,
		}), nil
	case store.OAuthProviderMicrosoft:
		return microsoftprov.New(microsoftprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: microsoftScopes, Tenant: "common", HTTPClient: hc,
			AuthorizeURL: ep.MicrosoftAuthorize, TokenURL: ep.MicrosoftToken,
		}), nil
	default:
		dctx, cancel := context.WithTimeout(ctx, oidcDiscoveryTimeout)
		defer cancel()
		p, err := oidcprov.New(dctx, oidcprov.Config{
			Name: store.OAuthProviderOIDC, Issuer: s.IssuerURL, ClientID: s.ClientID, ClientSecret: secret,
			Scopes: oidcScopes, AllowInsecureHTTP: ep.AllowInsecureOIDC, HTTPClient: hc,
		})
		if err != nil {
			return nil, fmt.Errorf("authengine: oidc discovery for provider %s: %w", s.Provider, err)
		}
		return p, nil
	}
}
