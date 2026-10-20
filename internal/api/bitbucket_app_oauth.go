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

// bitbucketAppCallbackPath must match the redirect_uri sent to Bitbucket's authorize endpoint exactly.
const bitbucketAppCallbackPath = "/api/v1/bitbucket-app/callback"

// handleStartBitbucketAppConnect starts the OAuth2 flow with a plain browser redirect.
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
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting bitbucket: it needs a real, reachable redirect url")
			return
		}
		rt.logger.Error("api: get ingress settings for bitbucket app connect failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	state, err := rt.bitbucketAppState.begin("")
	if err != nil {
		rt.logger.Error("api: begin bitbucket app oauth state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	authorizeURL := bitbucketapp.AuthorizeURL(conn.Key, baseURL+bitbucketAppCallbackPath, state)
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// handleBitbucketAppCallback completes the OAuth2 flow from the returned code and state.
func (rt *Router) handleBitbucketAppCallback(w http.ResponseWriter, r *http.Request) {
	rt.invalidateGitRepoCache(r.Context(), gitProviderBitbucket)
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
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	secretsKey := store.BitbucketAppSecretsKey()
	consumerSecret, err := rt.bitbucketAppSecrets.Resolve(ctx, secretsKey, bitbucketAppSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve bitbucket app secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
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
		writeError(w, http.StatusInternalServerError, errInternal)
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

// bitbucketTokenRefreshMargin is tighter than GitLab's, since Bitbucket's 2-hour tokens make refresh the normal path.
const bitbucketTokenRefreshMargin = 60 * time.Second

// bitbucketAccessToken returns only the token, since Bitbucket Cloud has no instance URL to carry back.
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

	return rt.refreshGitOAuth(ctx, gitOAuthSpec{
		provider: gitProviderBitbucket,
		read: func(ctx context.Context) (gitOAuthState, error) {
			return rt.readGitOAuthState(ctx, rt.bitbucketAppSecrets, key, bitbucketAppAccessTokenKey, bitbucketAppRefreshTokenKey, bitbucketAppTokenExpiresKey)
		},
		exchange: func(ctx context.Context, refresh string) (gitOAuthTokens, error) {
			consumerSecret, err := rt.bitbucketAppSecrets.Resolve(ctx, key, bitbucketAppSecretKey)
			if err != nil {
				return gitOAuthTokens{}, fmt.Errorf("resolve bitbucket consumer secret: %w", err)
			}
			t, err := rt.bitbucketAppClient.RefreshToken(ctx, conn.Key, consumerSecret, refresh)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		save: func(ctx context.Context, t gitOAuthTokens) error {
			return rt.saveBitbucketTokens(ctx, bitbucketapp.Tokens{AccessToken: t.Access, RefreshToken: t.Refresh, ExpiresAt: t.Expires})
		},
	})
}

// writeBitbucketAppTokenError maps bitbucketAccessToken's sentinels to
// 409, everything else to a logged 500, mirroring
// writeGitLabAppTokenError.
func (rt *Router) writeBitbucketAppTokenError(w http.ResponseWriter, logMsg string, err error) {
	if rt.writeGitConnError(w, err) {
		return
	}
	if errors.Is(err, errBitbucketAppNotConnected) {
		writeError(w, http.StatusConflict, "no bitbucket app is connected")
		return
	}
	if errors.Is(err, errBitbucketAppNotAuthorized) {
		writeError(w, http.StatusConflict, "the bitbucket app is connected but not yet authorized; connect it first")
		return
	}
	rt.logger.Error(logMsg, slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, errInternal)
}
