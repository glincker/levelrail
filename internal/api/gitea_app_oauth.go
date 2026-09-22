package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// giteaAppCallbackPath is this control plane's own OAuth redirect_uri
// path, registered in routes_platform.go. Must match exactly what
// handleStartGiteaAppConnect sends Gitea's authorize endpoint and what
// handleGiteaAppCallback sends the token endpoint, the same reasoning
// gitlabAppCallbackPath's own doc comment gives.
const giteaAppCallbackPath = "/api/v1/gitea-app/callback"

// handleStartGiteaAppConnect handles GET /api/v1/gitea-app/connect: the
// entry point for Gitea's OAuth2 authorization-code flow. A plain
// redirect, the same shape handleStartGitLabAppConnect's own doc
// comment describes.
func (rt *Router) handleStartGiteaAppConnect(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}

	ctx := r.Context()
	conn, err := rt.giteaApp.GetGiteaAppConnection(ctx)
	if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "configure the gitea oauth application first (instance url, client id, client secret)")
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitea app connection for connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting gitea: it needs a real, reachable redirect url")
			return
		}
		rt.logger.Error("api: get ingress settings for gitea app connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	state, err := rt.giteaAppState.begin("")
	if err != nil {
		rt.logger.Error("api: begin gitea app oauth state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	authorizeURL := giteaapp.AuthorizeURL(conn.InstanceURL, conn.ClientID, baseURL+giteaAppCallbackPath, state)
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// handleGiteaAppCallback handles GET /api/v1/gitea-app/callback: Gitea's
// redirect back once the operator approves the authorization request,
// carrying ?code=...&state=.... Mirrors handleGitLabAppCallback's own
// state/code validation and "never log a credential value" rule.
func (rt *Router) handleGiteaAppCallback(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}

	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("code"))
	state := strings.TrimSpace(q.Get("state"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code parameter")
		return
	}
	if state == "" {
		writeError(w, http.StatusBadRequest, "missing state parameter")
		return
	}
	if _, ok := rt.giteaAppState.consume(state); !ok {
		writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
		return
	}

	ctx := r.Context()
	conn, err := rt.giteaApp.GetGiteaAppConnection(ctx)
	if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "no gitea oauth application is configured on this control plane")
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitea app connection for callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	secretsKey := store.GiteaAppSecretsKey()
	clientSecret, err := rt.giteaAppSecrets.Resolve(ctx, secretsKey, giteaAppClientSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve gitea app client_secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		rt.logger.Error("api: get base url for gitea app callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	tokens, err := rt.giteaAppClient.ExchangeCode(ctx, conn.InstanceURL, conn.ClientID, clientSecret, baseURL+giteaAppCallbackPath, code)
	if err != nil {
		rt.logger.Error("api: exchange gitea oauth code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to exchange the authorization code with gitea")
		return
	}
	if err := rt.saveGiteaTokens(ctx, tokens); err != nil {
		rt.logger.Error("api: save gitea oauth tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	http.Redirect(w, r, baseURL+"/settings/gitea-app?gitea_app=connected", http.StatusFound)
}

// saveGiteaTokens persists an access/refresh token pair, storing the
// expiry as a Unix timestamp string, mirroring saveGitLabTokens exactly.
func (rt *Router) saveGiteaTokens(ctx context.Context, tokens giteaapp.Tokens) error {
	key := store.GiteaAppSecretsKey()
	if err := rt.giteaAppSecrets.SetValue(ctx, key, giteaAppAccessTokenKey, tokens.AccessToken); err != nil {
		return fmt.Errorf("save access_token: %w", err)
	}
	if tokens.RefreshToken != "" {
		if err := rt.giteaAppSecrets.SetValue(ctx, key, giteaAppRefreshTokenKey, tokens.RefreshToken); err != nil {
			return fmt.Errorf("save refresh_token: %w", err)
		}
	}
	if err := rt.giteaAppSecrets.SetValue(ctx, key, giteaAppTokenExpiresKey, strconv.FormatInt(tokens.ExpiresAt.Unix(), 10)); err != nil {
		return fmt.Errorf("save token_expires_at: %w", err)
	}
	return nil
}

// errGiteaAppNotConnected and errGiteaAppNotAuthorized are
// giteaAccessToken's own sentinels, mapped to 409 by
// writeGiteaAppTokenError below, mirroring
// errGitLabAppNotConnected/errGitLabAppNotAuthorized.
var (
	errGiteaAppNotConnected  = errors.New("api: gitea app is not connected")
	errGiteaAppNotAuthorized = errors.New("api: gitea app is connected but not authorized")
)

// giteaTokenRefreshMargin mirrors gitLabTokenRefreshMargin's own value
// and reasoning.
const giteaTokenRefreshMargin = 60 * time.Second

// giteaAccessToken returns a valid access token for conn, refreshing it
// first if the stored one is at or past its recorded expiry. Mirrors
// gitlabAccessToken's own refresh logic and return shape exactly:
// Gitea, like GitLab, is self-hosted, so every caller needs conn back
// for its InstanceURL.
func (rt *Router) giteaAccessToken(ctx context.Context) (store.GiteaAppConnection, string, error) {
	conn, err := rt.giteaApp.GetGiteaAppConnection(ctx)
	if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
		return store.GiteaAppConnection{}, "", errGiteaAppNotConnected
	}
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("get gitea app connection: %w", err)
	}

	key := store.GiteaAppSecretsKey()
	authorized, err := rt.giteaAppSecrets.Exists(ctx, key, giteaAppAccessTokenKey)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("check gitea access token: %w", err)
	}
	if !authorized {
		return store.GiteaAppConnection{}, "", errGiteaAppNotAuthorized
	}

	expiresAtRaw, err := rt.giteaAppSecrets.Resolve(ctx, key, giteaAppTokenExpiresKey)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("resolve gitea token expiry: %w", err)
	}
	expiresAtUnix, err := strconv.ParseInt(expiresAtRaw, 10, 64)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("parse gitea token expiry: %w", err)
	}

	if time.Now().Add(giteaTokenRefreshMargin).Before(time.Unix(expiresAtUnix, 0)) {
		accessToken, err := rt.giteaAppSecrets.Resolve(ctx, key, giteaAppAccessTokenKey)
		if err != nil {
			return store.GiteaAppConnection{}, "", fmt.Errorf("resolve gitea access token: %w", err)
		}
		return conn, accessToken, nil
	}

	refreshToken, err := rt.giteaAppSecrets.Resolve(ctx, key, giteaAppRefreshTokenKey)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("resolve gitea refresh token: %w", err)
	}
	clientSecret, err := rt.giteaAppSecrets.Resolve(ctx, key, giteaAppClientSecretKey)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("resolve gitea client secret: %w", err)
	}
	tokens, err := rt.giteaAppClient.RefreshToken(ctx, conn.InstanceURL, conn.ClientID, clientSecret, refreshToken)
	if err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("refresh gitea access token: %w", err)
	}
	if err := rt.saveGiteaTokens(ctx, tokens); err != nil {
		return store.GiteaAppConnection{}, "", fmt.Errorf("save refreshed gitea tokens: %w", err)
	}
	return conn, tokens.AccessToken, nil
}

// writeGiteaAppTokenError maps giteaAccessToken's sentinels to 409,
// everything else to a logged 500, mirroring writeGitLabAppTokenError.
func (rt *Router) writeGiteaAppTokenError(w http.ResponseWriter, logMsg string, err error) {
	if errors.Is(err, errGiteaAppNotConnected) {
		writeError(w, http.StatusConflict, "no gitea app is connected")
		return
	}
	if errors.Is(err, errGiteaAppNotAuthorized) {
		writeError(w, http.StatusConflict, "the gitea app is connected but not yet authorized; connect it first")
		return
	}
	rt.logger.Error(logMsg, slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, errInternal)
}
