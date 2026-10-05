package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

// AuthLibOAuth is the slice of the library auth engine the OAuth sign-in routes drive.
type AuthLibOAuth interface {
	OAuthEnabled() bool
	Prefix() string
	OAuthStart(r *http.Request, provider string) authengine.OAuthOutcome
	OAuthCallback(r *http.Request, provider string) authengine.OAuthOutcome
	ResolveOAuthUser(ctx context.Context, provider, providerUserID string) (string, error)
	LinkOAuthIdentity(ctx context.Context, legacyUserID, provider, providerUserID string) error
	InvalidateProvider(name string)
}

// WithAuthLibOAuth serves OAuth and OIDC sign-in through the library engine when
// APP_AUTH_ENGINE_AREAS includes oauth. A nil engine keeps the in-house flow.
func WithAuthLibOAuth(e AuthLibOAuth) Option {
	return func(rt *Router) { rt.authLibOAuth = e }
}

func (rt *Router) authLibOAuthActive() bool {
	return rt.authLibOAuth != nil && rt.authLibOAuth.OAuthEnabled() && authengine.AreaActive(authengine.AreaOAuth)
}

func (rt *Router) authLibOAuthInvalidate(provider string) {
	if rt.authLibOAuthActive() {
		rt.authLibOAuth.InvalidateProvider(provider)
	}
}

// authLibOAuthMirrorLink keeps the library's account table in step with an identity
// linked through the in-house link flow.
func (rt *Router) authLibOAuthMirrorLink(ctx context.Context, userID, provider, providerUserID string) {
	if !rt.authLibOAuthActive() {
		return
	}
	if err := rt.authLibOAuth.LinkOAuthIdentity(ctx, userID, provider, providerUserID); err != nil {
		rt.logger.Warn("api: mirror linked oauth identity failed", slog.String("provider", provider), slog.String("user_id", userID), slog.String("error", err.Error()))
	}
}

func (rt *Router) registerAuthLibOAuthRoutes(mux *http.ServeMux) {
	if rt.authLibOAuth == nil || !rt.authLibOAuthActive() {
		return
	}
	mux.HandleFunc("GET "+rt.authLibOAuth.Prefix()+"/providers/{provider}/callback", rt.authLibOAuthCallback)
}

// forwardOAuthCookies relays the library's flow cookies scoped to this provider's callback
// and Secure whenever the client connection is HTTPS, like the in-house binding cookie.
func (rt *Router) forwardOAuthCookies(w http.ResponseWriter, r *http.Request, provider string, cookies []*http.Cookie) {
	for _, c := range cookies {
		c.Path = authengine.OAuthCallbackPath(provider)
		c.Secure = requestIsHTTPS(r)
		http.SetCookie(w, c) // NOSONAR: Secure follows the client transport, see requestIsHTTPS
	}
}

func (rt *Router) authLibOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return
	}
	out := rt.authLibOAuth.OAuthStart(r, provider)
	switch {
	case out.Status == http.StatusFound && out.Location != "":
		rt.forwardOAuthCookies(w, r, provider, out.Cookies)
		http.Redirect(w, r, out.Location, http.StatusFound)
	case out.Status == http.StatusNotFound:
		writeError(w, http.StatusBadRequest, "this provider is not enabled")
	case out.Status == http.StatusServiceUnavailable:
		rt.logger.Error("api: oauth start: provider unavailable", slog.String("provider", provider))
		writeError(w, http.StatusInternalServerError, "oauth provider is misconfigured")
	default:
		rt.logger.Error("api: oauth start failed", slog.String("provider", provider), slog.Int("status", out.Status))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (rt *Router) authLibOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		redirectOAuthError(w, r, "invalid_provider")
		return
	}
	out := rt.authLibOAuth.OAuthCallback(r, provider)
	rt.forwardOAuthCookies(w, r, provider, out.Cookies)
	if out.ErrorCode != "" {
		rt.logger.Warn("api: oauth callback failed", slog.String("provider", provider), slog.String("code", out.ErrorCode))
		redirectOAuthError(w, r, out.ErrorCode)
		return
	}
	userID, err := rt.authLibOAuth.ResolveOAuthUser(r.Context(), out.Provider, out.ProviderUserID)
	if err != nil {
		rt.logger.Error("api: oauth callback: resolve user failed", slog.String("provider", provider), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}
	if authengine.AreaActive(authengine.AreaSessions) {
		http.SetCookie(w, out.SessionCookie) // NOSONAR: attributes come from the library session cookie
		http.Redirect(w, r, "/oauth/complete", http.StatusFound)
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err == nil {
		err = rt.establishSession(w, r, *user)
	}
	if err != nil {
		rt.logger.Error("api: oauth callback: establish session failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		redirectOAuthError(w, r, "internal_error")
		return
	}
	http.Redirect(w, r, "/oauth/complete", http.StatusFound)
}
