package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/oauth2"
)

// OAuthSettingsStore is the store surface the OAuth settings and sign-in
// handlers need for provider configuration (oauth_settings.go, this
// file). *store.DB satisfies this structurally.
type OAuthSettingsStore interface {
	GetOAuthProviderSettings(ctx context.Context, provider string) (store.OAuthProviderSettings, error)
	ListOAuthProviderSettings(ctx context.Context) ([]store.OAuthProviderSettings, error)
	UpdateOAuthProviderSettings(ctx context.Context, s store.OAuthProviderSettings) error
}

// OAuthIdentityStore is the store surface the sign-in and account-
// linking handlers need. *store.DB satisfies this structurally.
type OAuthIdentityStore interface {
	GetOAuthIdentity(ctx context.Context, provider, providerUserID string) (*store.OAuthIdentity, error)
	SaveOAuthIdentity(ctx context.Context, i store.OAuthIdentity) error
	ListOAuthIdentitiesForUser(ctx context.Context, userID string) ([]store.OAuthIdentity, error)
}

// OAuthSecrets is the surface the OAuth flow needs from
// internal/secrets.Manager: unlike SecretSetter (write-only), a
// provider's client secret must be resolved on every sign-in attempt.
type OAuthSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// WithOAuthSecrets enables OAuth sign-in end to end. Without one
// configured (the default), those routes return 501, the same
// "not configured" shape WithSecretSetter's absence produces.
func WithOAuthSecrets(s OAuthSecrets) Option {
	return func(rt *Router) { rt.oauthSecrets = s }
}

func isValidOAuthProvider(p string) bool {
	return p == store.OAuthProviderGoogle || p == store.OAuthProviderGitHub || p == store.OAuthProviderOIDC || p == store.OAuthProviderMicrosoft
}

// Sentinel errors completeOAuthSignin/completeOAuthLink return, mapped
// to short, detail-free redirect codes by oauthErrorCode.
var ()

// oauthBindingCookieName ties an OAuth state to the browser that started
// the flow, defeating login CSRF with an attacker-initiated state.
const oauthBindingCookieName = "oauth_binding"

func setOAuthBindingCookie(w http.ResponseWriter, r *http.Request, nonce string, ttl time.Duration) {
	c := &http.Cookie{ //nolint:gosec // Secure follows the transport, see requestIsHTTPS
		Name:     oauthBindingCookieName,
		Value:    nonce,
		Path:     "/api/v1/auth/oauth",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	}
	if nonce == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c) // NOSONAR: Secure is set whenever the client connection is HTTPS, see requestIsHTTPS
}

// handleListPublicOAuthProviders handles GET /api/v1/auth/oauth/providers:
// public, unauthenticated. Reveals only (provider, enabled), never a
// client ID or secret.
func (rt *Router) handleListPublicOAuthProviders(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.oauthSettings.ListOAuthProviderSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: list public oauth providers failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	type publicProvider struct {
		Provider    string `json:"provider"`
		Enabled     bool   `json:"enabled"`
		DisplayName string `json:"display_name,omitempty"`
	}
	out := make([]publicProvider, 0, len(settings))
	for _, s := range settings {
		out = append(out, publicProvider{Provider: s.Provider, Enabled: s.Enabled, DisplayName: s.DisplayName})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleOAuthStart handles GET /api/v1/auth/oauth/{provider}/start: the
// public, anonymous sign-in entry point.
func (rt *Router) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if !rt.authLibOAuthActive() {
		writeError(w, http.StatusNotImplemented, "oauth sign-in is not configured")
		return
	}
	rt.authLibOAuthStart(w, r)
}

// handleOAuthLinkStart handles GET /api/v1/auth/oauth/{provider}/link/start:
// requireAuth-gated, the answer to the account-linking security
// question: attaching a new OAuth identity to an existing account only
// ever happens through here, with the caller already proven to own that
// account via a live session. The user ID is captured into server-side
// oauthState now, not re-derived from a cookie on the later callback.
func (rt *Router) handleOAuthLinkStart(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rt.beginOAuthFlow(w, r, oauthPurposeLink, userID)
}

func (rt *Router) beginOAuthFlow(w http.ResponseWriter, r *http.Request, purpose, linkUserID string) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return
	}
	if rt.oauthSettings == nil || rt.oauthSecrets == nil {
		writeError(w, http.StatusNotImplemented, "oauth sign-in is not configured")
		return
	}

	settings, err := rt.oauthSettings.GetOAuthProviderSettings(r.Context(), provider)
	if err != nil {
		rt.logger.Error("api: oauth start: load settings failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !settings.Enabled {
		writeError(w, http.StatusBadRequest, "this provider is not enabled")
		return
	}

	clientSecret, err := rt.oauthSecrets.Resolve(r.Context(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey)
	if err != nil {
		rt.logger.Error("api: oauth start: resolve client secret failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "oauth provider is misconfigured")
		return
	}

	client, err := rt.oauthClientFactory(provider, settings, clientSecret, oauthRedirectURL(r, provider))
	if err != nil {
		rt.logger.Error("api: oauth start: build client failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	nonce, err := randomToken()
	if err != nil {
		rt.logger.Error("api: oauth start: generate nonce failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	verifier := oauth2.GenerateVerifier()
	state, err := rt.oauthState.create(provider, purpose, linkUserID, nonce, verifier)
	if err != nil {
		rt.logger.Error("api: oauth start: generate state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	setOAuthBindingCookie(w, r, nonce, oauthStateTTL)
	http.Redirect(w, r, client.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// handleOAuthCallback handles GET /api/v1/auth/oauth/{provider}/callback,
// shared by sign-in and link flows (the consumed state carries which
// purpose). Never returns a JSON error: every failure redirects to
// /login with a short error code, since this is reached via a real
// browser navigation, not a fetch.
func (rt *Router) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		redirectOAuthError(w, r, "invalid_provider")
		return
	}
	if rt.oauthSettings == nil || rt.oauthSecrets == nil {
		redirectOAuthError(w, r, "not_configured")
		return
	}

	q := r.URL.Query()
	if q.Get("error") != "" {
		redirectOAuthError(w, r, "provider_denied")
		return
	}
	stateToken, code := q.Get("state"), q.Get("code")
	if stateToken == "" || code == "" {
		redirectOAuthError(w, r, "invalid_request")
		return
	}

	st, ok := rt.oauthState.consume(stateToken)
	if !ok || st.provider != provider {
		redirectOAuthError(w, r, "invalid_state")
		return
	}
	cookie, cerr := r.Cookie(oauthBindingCookieName)
	setOAuthBindingCookie(w, r, "", 0)
	if cerr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(st.nonce)) != 1 {
		redirectOAuthError(w, r, "invalid_state")
		return
	}

	settings, err := rt.oauthSettings.GetOAuthProviderSettings(r.Context(), provider)
	if err != nil {
		rt.logger.Error("api: oauth callback: load settings failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}
	if !settings.Enabled {
		redirectOAuthError(w, r, "provider_disabled")
		return
	}

	clientSecret, err := rt.oauthSecrets.Resolve(r.Context(), store.OAuthProviderSecretsKey(provider), store.OAuthProviderSecretEnvKey)
	if err != nil {
		rt.logger.Error("api: oauth callback: resolve client secret failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}

	client, err := rt.oauthClientFactory(provider, settings, clientSecret, oauthRedirectURL(r, provider))
	if err != nil {
		rt.logger.Error("api: oauth callback: build client failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}

	token, err := client.Exchange(r.Context(), code, oauth2.VerifierOption(st.verifier))
	if err != nil {
		rt.logger.Warn("api: oauth code exchange failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "exchange_failed")
		return
	}
	info, err := client.FetchUserInfo(r.Context(), token)
	if err != nil {
		rt.logger.Warn("api: oauth userinfo fetch failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "userinfo_failed")
		return
	}

	if st.purpose != oauthPurposeLink {
		redirectOAuthError(w, r, "invalid_state")
		return
	}
	user, err := rt.completeOAuthLink(r.Context(), st.linkUserID, provider, info)
	if err != nil {
		rt.logger.Warn("api: oauth callback failed", slog.String("provider", provider), slog.String("purpose", st.purpose), slog.String("error", err.Error()))
		redirectOAuthError(w, r, oauthErrorCode(err))
		return
	}

	if err := rt.establishSession(w, r, user); err != nil {
		rt.logger.Error("api: oauth callback: establish session failed", slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}
	http.Redirect(w, r, "/oauth/complete", http.StatusFound)
}

// completeOAuthLink attaches a new external identity to the user who
// initiated this flow (linkUserID, captured server-side by
// handleOAuthLinkStart, not read from this request).
// store.SaveOAuthIdentity's unique indexes reject an already-linked
// identity as ErrOAuthIdentityAlreadyLinked.
func (rt *Router) completeOAuthLink(ctx context.Context, linkUserID, provider string, info oauthUserInfo) (store.User, error) {
	user, err := rt.auth.GetUserByID(ctx, linkUserID)
	if err != nil {
		return store.User{}, fmt.Errorf("load linking user: %w", err)
	}

	identityID, err := randomOpaqueID("oid_")
	if err != nil {
		return store.User{}, err
	}
	if err := rt.oauthIdentities.SaveOAuthIdentity(ctx, store.OAuthIdentity{
		ID: identityID, UserID: user.ID, Provider: provider, ProviderUserID: info.ProviderUserID, CreatedAt: time.Now(),
	}); err != nil {
		if errors.Is(err, store.ErrOAuthIdentityAlreadyLinked) {
			return store.User{}, err
		}
		return store.User{}, fmt.Errorf("save oauth identity: %w", err)
	}
	rt.authLibOAuthMirrorLink(ctx, user.ID, provider, info.ProviderUserID)
	return *user, nil
}

func oauthErrorCode(err error) string {
	if errors.Is(err, store.ErrOAuthIdentityAlreadyLinked) {
		return "already_linked"
	}
	return "signin_failed"
}

func redirectOAuthError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?oauth_error="+code, http.StatusFound)
}

// oauthRedirectURL builds this callback's own absolute URL from the
// incoming request rather than a fixed setting: a registered OAuth
// redirect URI must exactly match, and the request's own Host is always
// correct for whatever domain the operator is reaching this through.
func oauthRedirectURL(r *http.Request, provider string) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/v1/auth/oauth/" + provider + "/callback"
}
