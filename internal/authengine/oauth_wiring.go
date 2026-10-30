package authengine

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	theauth "github.com/glincker/theauth-go/v2"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// EnvOAuthProviderTTL overrides how long a resolved OAuth provider is cached (Go duration).
	EnvOAuthProviderTTL = "APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL"

	defaultOAuthProviderTTL = 30 * time.Second
)

// OAuthSettingsSource reads the dashboard-editable provider settings (*store.DB).
type OAuthSettingsSource interface {
	GetOAuthProviderSettings(ctx context.Context, provider string) (store.OAuthProviderSettings, error)
	ListOAuthProviderSettings(ctx context.Context) ([]store.OAuthProviderSettings, error)
}

// LegacyUserWriter creates and reads platform users and their linked identities (*store.DB).
type LegacyUserWriter interface {
	CreateProvisionedUser(ctx context.Context, u store.User, roleID string) error
	SaveOAuthIdentity(ctx context.Context, i store.OAuthIdentity) error
}

// OAuthEndpoints overrides provider endpoints so tests can point the library at a fake IdP.
// Production leaves every field empty.
type OAuthEndpoints struct {
	GoogleAuthorize, GoogleToken, GoogleUserInfo           string
	GitHubAuthorize, GitHubToken, GitHubUser, GitHubEmails string
	MicrosoftAuthorize, MicrosoftToken, MicrosoftGraphMe   string
	AllowInsecureOIDC                                      bool
}

// OAuthWiring turns on OAuth and OIDC sign-in through the library. Leave
// Config.OAuth nil to keep it off.
type OAuthWiring struct {
	Settings  OAuthSettingsSource
	Secrets   SecretResolver
	Users     LegacyUserWriter
	Endpoints OAuthEndpoints
	HTTP      *http.Client
	Logger    *slog.Logger
	// TrustedPeer says whether a request's TCP peer may set Host and X-Forwarded-Proto
	// when no dashboard URL is configured. Nil means loopback only.
	TrustedPeer func(r *http.Request) bool
}

type oauthRuntime struct {
	wiring   OAuthWiring
	db       *sql.DB
	resolver *providerResolver
	key      []byte
	redirect OAuthRedirectResolver
}

func oauthProviderTTL() time.Duration {
	if v := os.Getenv(EnvOAuthProviderTTL); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			return d
		}
	}
	return defaultOAuthProviderTTL
}

// applyOAuth wires the resolver and signup policy into tcfg. It returns nil
// (OAuth stays off) when no wiring or no encryption key is configured.
func applyOAuth(tcfg *theauth.Config, cfg Config, db *sql.DB) *oauthRuntime {
	w := cfg.OAuth
	if w == nil || w.Settings == nil || w.Secrets == nil || w.Users == nil || len(cfg.EncryptionKey) == 0 {
		return nil
	}
	if w.Logger == nil {
		w.Logger = slog.Default()
	}
	rt := &oauthRuntime{wiring: *w, db: db, key: cfg.EncryptionKey}
	rt.redirect = OAuthRedirectResolver{
		DashboardURL: func(ctx context.Context) string {
			if u := LoadDashboardURL(ctx, db); u != "" {
				return u
			}
			return cfg.MFA.DashboardURL
		},
		TrustedPeer: w.TrustedPeer,
	}
	rt.resolver = newProviderResolver(rt)
	tcfg.ProviderResolver = rt.resolver
	tcfg.ProviderResolverTTL = oauthProviderTTL()
	tcfg.OAuth = &theauth.OAuthConfig{
		Signup:                  theauth.OAuthSignupOpen,
		RedirectURI:             rt.redirect.Resolve,
		RedirectURIAllowedHosts: oauthAllowedHosts(cfg),
		// Fresh installs sign in over HTTP until an https dashboard URL is set, as the built-in flow allowed.
		AllowInsecureRedirectURI: true,
	}
	return rt
}

// OAuthEnabled reports whether the library serves OAuth sign-in for this engine.
func (e *Engine) OAuthEnabled() bool { return e.oauth != nil }

// OAuthRedirectURI resolves the provider redirect URI the same way the library flow does.
func (e *Engine) OAuthRedirectURI(r *http.Request, provider string) (string, error) {
	if e.oauth == nil {
		return OAuthRedirectResolver{}.Resolve(r, provider)
	}
	return e.oauth.redirect.Resolve(r, provider)
}

// InvalidateProvider drops the cached provider so a settings edit applies on the next request.
func (e *Engine) InvalidateProvider(name string) {
	e.auth.InvalidateProvider(name)
}
