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

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gitlabAppCallbackPath is this control plane's own OAuth redirect_uri
// path, registered in router.go. Must match exactly what
// handleStartGitLabAppConnect sends GitLab's authorize endpoint and what
// handleGitLabAppCallback sends the token endpoint: GitLab, like every
// OAuth2 provider, rejects a code exchange whose redirect_uri doesn't
// match the one the authorize request used.
const gitlabAppCallbackPath = "/api/v1/gitlab-app/callback"

// handleStartGitLabAppConnect handles GET /api/v1/gitlab-app/connect:
// the entry point for GitLab's OAuth2 authorization-code flow. Unlike
// GitHub's manifest flow, this is a plain redirect, no self-submitting
// form needed: GitLab's own /oauth/authorize endpoint accepts a GET.
func (rt *Router) handleStartGitLabAppConnect(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	conn, err := rt.gitlabApp.GetGitLabAppConnection(ctx)
	if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "configure the gitlab oauth application first (instance url, client id, client secret)")
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitlab app connection for connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting gitlab: it needs a real, reachable redirect url")
			return
		}
		rt.logger.Error("api: get ingress settings for gitlab app connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	state, err := rt.gitlabAppState.begin()
	if err != nil {
		rt.logger.Error("api: begin gitlab app oauth state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	authorizeURL := gitlabapp.AuthorizeURL(conn.InstanceURL, conn.ClientID, baseURL+gitlabAppCallbackPath, state)
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// handleGitLabAppCallback handles GET /api/v1/gitlab-app/callback:
// GitLab's redirect back once the operator approves the authorization
// request, carrying ?code=...&state=.... Mirrors
// handleGitHubAppCallback's own state/code validation and "never log a
// credential value" rule.
func (rt *Router) handleGitLabAppCallback(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
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
	if !rt.gitlabAppState.consume(state) {
		writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
		return
	}

	ctx := r.Context()
	conn, err := rt.gitlabApp.GetGitLabAppConnection(ctx)
	if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "no gitlab oauth application is configured on this control plane")
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitlab app connection for callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	secretsKey := store.GitLabAppSecretsKey()
	clientSecret, err := rt.gitlabAppSecrets.Resolve(ctx, secretsKey, gitLabAppClientSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve gitlab app client_secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		rt.logger.Error("api: get base url for gitlab app callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tokens, err := rt.gitlabAppClient.ExchangeCode(ctx, conn.InstanceURL, conn.ClientID, clientSecret, baseURL+gitlabAppCallbackPath, code)
	if err != nil {
		rt.logger.Error("api: exchange gitlab oauth code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to exchange the authorization code with gitlab")
		return
	}
	if err := rt.saveGitLabTokens(ctx, tokens); err != nil {
		rt.logger.Error("api: save gitlab oauth tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.Redirect(w, r, baseURL+"/settings/gitlab-app?gitlab_app=connected", http.StatusFound)
}

// saveGitLabTokens persists an access/refresh token pair, storing the
// expiry as a Unix timestamp string so gitlabAccessToken can compare it
// against time.Now() without a parse format to get wrong.
func (rt *Router) saveGitLabTokens(ctx context.Context, tokens gitlabapp.Tokens) error {
	key := store.GitLabAppSecretsKey()
	if err := rt.gitlabAppSecrets.SetValue(ctx, key, gitLabAppAccessTokenKey, tokens.AccessToken); err != nil {
		return fmt.Errorf("save access_token: %w", err)
	}
	if tokens.RefreshToken != "" {
		if err := rt.gitlabAppSecrets.SetValue(ctx, key, gitLabAppRefreshTokenKey, tokens.RefreshToken); err != nil {
			return fmt.Errorf("save refresh_token: %w", err)
		}
	}
	if err := rt.gitlabAppSecrets.SetValue(ctx, key, gitLabAppTokenExpiresKey, strconv.FormatInt(tokens.ExpiresAt.Unix(), 10)); err != nil {
		return fmt.Errorf("save token_expires_at: %w", err)
	}
	return nil
}

// errGitLabAppNotConnected and errGitLabAppNotAuthorized are
// gitlabAccessToken's own sentinels, mapped to 409 by
// writeGitLabAppTokenError below, mirroring
// errGitHubAppNotConnected/errGitHubAppNotInstalled.
var (
	errGitLabAppNotConnected  = errors.New("api: gitlab app is not connected")
	errGitLabAppNotAuthorized = errors.New("api: gitlab app is connected but not authorized")
)

// gitLabTokenRefreshMargin is how long before a stored access token's
// recorded expiry gitlabAccessToken proactively refreshes it, so a
// project-listing or webhook-registration call in flight doesn't lose a
// race against the token expiring mid-request.
const gitLabTokenRefreshMargin = 60 * time.Second

// gitlabAccessToken returns a valid access token for conn, refreshing
// it first if the stored one is at or past its recorded expiry.
// Refreshing is best-effort transparent to every caller (project
// listing, project lookup, webhook registration): none of them need to
// know whether the token they got back is the one that was already
// stored or a freshly minted one.
func (rt *Router) gitlabAccessToken(ctx context.Context) (store.GitLabAppConnection, string, error) {
	conn, err := rt.gitlabApp.GetGitLabAppConnection(ctx)
	if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
		return store.GitLabAppConnection{}, "", errGitLabAppNotConnected
	}
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("get gitlab app connection: %w", err)
	}

	key := store.GitLabAppSecretsKey()
	authorized, err := rt.gitlabAppSecrets.Exists(ctx, key, gitLabAppAccessTokenKey)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("check gitlab access token: %w", err)
	}
	if !authorized {
		return store.GitLabAppConnection{}, "", errGitLabAppNotAuthorized
	}

	expiresAtRaw, err := rt.gitlabAppSecrets.Resolve(ctx, key, gitLabAppTokenExpiresKey)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("resolve gitlab token expiry: %w", err)
	}
	expiresAtUnix, err := strconv.ParseInt(expiresAtRaw, 10, 64)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("parse gitlab token expiry: %w", err)
	}

	if time.Now().Add(gitLabTokenRefreshMargin).Before(time.Unix(expiresAtUnix, 0)) {
		accessToken, err := rt.gitlabAppSecrets.Resolve(ctx, key, gitLabAppAccessTokenKey)
		if err != nil {
			return store.GitLabAppConnection{}, "", fmt.Errorf("resolve gitlab access token: %w", err)
		}
		return conn, accessToken, nil
	}

	refreshToken, err := rt.gitlabAppSecrets.Resolve(ctx, key, gitLabAppRefreshTokenKey)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("resolve gitlab refresh token: %w", err)
	}
	clientSecret, err := rt.gitlabAppSecrets.Resolve(ctx, key, gitLabAppClientSecretKey)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("resolve gitlab client secret: %w", err)
	}
	tokens, err := rt.gitlabAppClient.RefreshToken(ctx, conn.InstanceURL, conn.ClientID, clientSecret, refreshToken)
	if err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("refresh gitlab access token: %w", err)
	}
	if err := rt.saveGitLabTokens(ctx, tokens); err != nil {
		return store.GitLabAppConnection{}, "", fmt.Errorf("save refreshed gitlab tokens: %w", err)
	}
	return conn, tokens.AccessToken, nil
}

// writeGitLabAppTokenError maps gitlabAccessToken's sentinels to 409,
// everything else to a logged 500, mirroring writeGitHubAppTokenError.
func (rt *Router) writeGitLabAppTokenError(w http.ResponseWriter, logMsg string, err error) {
	if errors.Is(err, errGitLabAppNotConnected) {
		writeError(w, http.StatusConflict, "no gitlab app is connected")
		return
	}
	if errors.Is(err, errGitLabAppNotAuthorized) {
		writeError(w, http.StatusConflict, "the gitlab app is connected but not yet authorized; connect it first")
		return
	}
	rt.logger.Error(logMsg, slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, "internal error")
}
