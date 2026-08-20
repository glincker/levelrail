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

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// bitbucketAppCallbackPath is this control plane's own OAuth
// redirect_uri path, registered in routes_platform.go. Must match
// exactly what handleStartBitbucketAppConnect sends Bitbucket's
// authorize endpoint, the same reasoning gitlabAppCallbackPath's own
// doc comment gives.
const bitbucketAppCallbackPath = "/api/v1/bitbucket-app/callback"

// handleStartBitbucketAppConnect handles GET /api/v1/bitbucket-app/connect:
// the entry point for Bitbucket's OAuth2 authorization-code flow, a
// plain browser redirect, the same shape handleStartGitLabAppConnect's
// own doc comment describes for GitLab.
func (rt *Router) handleStartBitbucketAppConnect(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the bitbucket app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	conn, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx)
	if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "configure the bitbucket oauth consumer first (key, secret)")
		return
	}
	if err != nil {
		rt.logger.Error("api: get bitbucket app connection for connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting bitbucket: it needs a real, reachable redirect url")
			return
		}
		rt.logger.Error("api: get ingress settings for bitbucket app connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	state, err := rt.bitbucketAppState.begin("")
	if err != nil {
		rt.logger.Error("api: begin bitbucket app oauth state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	authorizeURL := bitbucketapp.AuthorizeURL(conn.Key, baseURL+bitbucketAppCallbackPath, state)
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// handleBitbucketAppCallback handles GET /api/v1/bitbucket-app/callback:
// Bitbucket's redirect back once the operator approves the
// authorization request, carrying ?code=...&state=.... Mirrors
// handleGitLabAppCallback's own state/code validation.
func (rt *Router) handleBitbucketAppCallback(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the bitbucket app connection requires a master key to be configured on this control plane")
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
	if _, ok := rt.bitbucketAppState.consume(state); !ok {
		writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
		return
	}

	ctx := r.Context()
	conn, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx)
	if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "no bitbucket oauth consumer is configured on this control plane")
		return
	}
	if err != nil {
		rt.logger.Error("api: get bitbucket app connection for callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	secretsKey := store.BitbucketAppSecretsKey()
	consumerSecret, err := rt.bitbucketAppSecrets.Resolve(ctx, secretsKey, bitbucketAppSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve bitbucket app secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tokens, err := rt.bitbucketAppClient.ExchangeCode(ctx, conn.Key, consumerSecret, code)
	if err != nil {
		rt.logger.Error("api: exchange bitbucket oauth code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to exchange the authorization code with bitbucket")
		return
	}
	if err := rt.saveBitbucketTokens(ctx, tokens); err != nil {
		rt.logger.Error("api: save bitbucket oauth tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "authorized"})
		return
	}
	http.Redirect(w, r, baseURL+"/settings/bitbucket-app?bitbucket_app=connected", http.StatusFound)
}

// saveBitbucketTokens persists an access/refresh token pair, storing the
// expiry as a Unix timestamp string, mirroring saveGitLabTokens exactly.
func (rt *Router) saveBitbucketTokens(ctx context.Context, tokens bitbucketapp.Tokens) error {
	key := store.BitbucketAppSecretsKey()
	if err := rt.bitbucketAppSecrets.SetValue(ctx, key, bitbucketAppAccessTokenKey, tokens.AccessToken); err != nil {
		return fmt.Errorf("save access_token: %w", err)
	}
	if tokens.RefreshToken != "" {
		if err := rt.bitbucketAppSecrets.SetValue(ctx, key, bitbucketAppRefreshTokenKey, tokens.RefreshToken); err != nil {
			return fmt.Errorf("save refresh_token: %w", err)
		}
	}
	if err := rt.bitbucketAppSecrets.SetValue(ctx, key, bitbucketAppTokenExpiresKey, strconv.FormatInt(tokens.ExpiresAt.Unix(), 10)); err != nil {
		return fmt.Errorf("save token_expires_at: %w", err)
	}
	return nil
}

// errBitbucketAppNotConnected and errBitbucketAppNotAuthorized are
// bitbucketAccessToken's own sentinels, mirroring
// errGitLabAppNotConnected/errGitLabAppNotAuthorized.
var (
	errBitbucketAppNotConnected  = errors.New("api: bitbucket app is not connected")
	errBitbucketAppNotAuthorized = errors.New("api: bitbucket app is connected but not authorized")
)

// bitbucketTokenRefreshMargin mirrors gitLabTokenRefreshMargin's own
// value and reasoning, sized tighter relative to Bitbucket's own
// 2-hour access token lifetime (docs/design/git-provider-integrations.md
// section 3: refresh is the normal path here, not an edge case) the
// same way it already is relative to GitLab's own token lifetime.
const bitbucketTokenRefreshMargin = 60 * time.Second

// bitbucketAccessToken returns a valid access token for the connected
// consumer, refreshing it first if the stored one is at or past its
// recorded expiry. Mirrors gitlabAccessToken's own refresh logic, but
// returns only the token, not the connection: Bitbucket Cloud has no
// instance_url the way GitLab does (this package is Cloud-only, see
// internal/bitbucketapp's own doc comment), so no caller here has ever
// needed the connection value back, unlike gitlabAccessToken's own
// callers, which use conn.InstanceURL on every API call.
func (rt *Router) bitbucketAccessToken(ctx context.Context) (string, error) {
	conn, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx)
	if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
		return "", errBitbucketAppNotConnected
	}
	if err != nil {
		return "", fmt.Errorf("get bitbucket app connection: %w", err)
	}

	key := store.BitbucketAppSecretsKey()
	authorized, err := rt.bitbucketAppSecrets.Exists(ctx, key, bitbucketAppAccessTokenKey)
	if err != nil {
		return "", fmt.Errorf("check bitbucket access token: %w", err)
	}
	if !authorized {
		return "", errBitbucketAppNotAuthorized
	}

	expiresAtRaw, err := rt.bitbucketAppSecrets.Resolve(ctx, key, bitbucketAppTokenExpiresKey)
	if err != nil {
		return "", fmt.Errorf("resolve bitbucket token expiry: %w", err)
	}
	expiresAtUnix, err := strconv.ParseInt(expiresAtRaw, 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse bitbucket token expiry: %w", err)
	}

	if time.Now().Add(bitbucketTokenRefreshMargin).Before(time.Unix(expiresAtUnix, 0)) {
		accessToken, err := rt.bitbucketAppSecrets.Resolve(ctx, key, bitbucketAppAccessTokenKey)
		if err != nil {
			return "", fmt.Errorf("resolve bitbucket access token: %w", err)
		}
		return accessToken, nil
	}

	refreshToken, err := rt.bitbucketAppSecrets.Resolve(ctx, key, bitbucketAppRefreshTokenKey)
	if err != nil {
		return "", fmt.Errorf("resolve bitbucket refresh token: %w", err)
	}
	consumerSecret, err := rt.bitbucketAppSecrets.Resolve(ctx, key, bitbucketAppSecretKey)
	if err != nil {
		return "", fmt.Errorf("resolve bitbucket consumer secret: %w", err)
	}
	tokens, err := rt.bitbucketAppClient.RefreshToken(ctx, conn.Key, consumerSecret, refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh bitbucket access token: %w", err)
	}
	if err := rt.saveBitbucketTokens(ctx, tokens); err != nil {
		return "", fmt.Errorf("save refreshed bitbucket tokens: %w", err)
	}
	return tokens.AccessToken, nil
}

// writeBitbucketAppTokenError maps bitbucketAccessToken's sentinels to
// 409, everything else to a logged 500, mirroring
// writeGitLabAppTokenError.
func (rt *Router) writeBitbucketAppTokenError(w http.ResponseWriter, logMsg string, err error) {
	if errors.Is(err, errBitbucketAppNotConnected) {
		writeError(w, http.StatusConflict, "no bitbucket app is connected")
		return
	}
	if errors.Is(err, errBitbucketAppNotAuthorized) {
		writeError(w, http.StatusConflict, "the bitbucket app is connected but not yet authorized; connect it first")
		return
	}
	rt.logger.Error(logMsg, slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, "internal error")
}
