package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BitbucketAppStore is the store surface the Bitbucket App connection
// handlers need. *store.DB satisfies this structurally, the same "core
// store" shape GitHubAppStore/GitLabAppStore's own doc comments
// establish.
type BitbucketAppStore interface {
	GetBitbucketAppConnection(ctx context.Context) (store.BitbucketAppConnection, error)
	SaveBitbucketAppConnection(ctx context.Context, c store.BitbucketAppConnection) error
	DeleteBitbucketAppConnection(ctx context.Context) error
}

// BitbucketAppSecrets is the surface the Bitbucket App handlers need
// from internal/secrets.Manager, the same "writes and reads back
// through internal/secrets" shape GitLabAppSecrets's own doc comment
// describes: the consumer's own secret is set once at connect time,
// access_token/refresh_token/token_expires_at are written and re-read
// on the same schedule by bitbucketAccessToken.
type BitbucketAppSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// BitbucketAppClient is the surface internal/api needs from
// internal/bitbucketapp.Client: OAuth code exchange/refresh, and repo
// listing/lookup/webhook registration once connected. *bitbucketapp.Client
// satisfies this structurally; tests substitute a hand-written fake.
type BitbucketAppClient interface {
	ExchangeCode(ctx context.Context, key, secret, code string) (bitbucketapp.Tokens, error)
	RefreshToken(ctx context.Context, key, secret, refreshToken string) (bitbucketapp.Tokens, error)
	ListRepos(ctx context.Context, accessToken string) ([]bitbucketapp.Repo, error)
	GetRepo(ctx context.Context, accessToken, fullName string) (bitbucketapp.Repo, error)
	ListBranches(ctx context.Context, accessToken, fullName string) ([]bitbucketapp.Branch, error)
	CreateRepoWebhook(ctx context.Context, accessToken, fullName, hookURL, secret string) error
}

const (
	bitbucketAppSecretKey       = "secret"
	bitbucketAppAccessTokenKey  = "access_token"
	bitbucketAppRefreshTokenKey = "refresh_token"
	bitbucketAppTokenExpiresKey = "token_expires_at"
)

// bitbucketAppStatusResource is the wire shape for
// GET /api/v1/bitbucket-app. No secret fields, matching
// gitHubAppStatusResource/gitLabAppStatusResource's own rule.
type bitbucketAppStatusResource struct {
	Connected  bool   `json:"connected"`
	Key        string `json:"key,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	Authorized bool   `json:"authorized"`
	BaseURL    string `json:"base_url,omitempty"`
}

// handleGetBitbucketAppStatus handles GET /api/v1/bitbucket-app.
func (rt *Router) handleGetBitbucketAppStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	baseURL, _ := rt.controlPlaneBaseURL(ctx)

	conn, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx)
	if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
		writeJSON(w, http.StatusOK, bitbucketAppStatusResource{Connected: false, BaseURL: baseURL})
		return
	}
	if err != nil {
		rt.logger.Error("api: get bitbucket app connection status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resource := bitbucketAppStatusResource{
		Connected: true,
		Key:       conn.Key,
		CreatedAt: conn.CreatedAt,
		BaseURL:   baseURL,
	}
	if rt.bitbucketAppSecrets != nil {
		authorized, err := rt.bitbucketAppSecrets.Exists(ctx, store.BitbucketAppSecretsKey(), bitbucketAppAccessTokenKey)
		if err != nil {
			rt.logger.Error("api: check bitbucket app authorization failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resource.Authorized = authorized
	}
	writeJSON(w, http.StatusOK, resource)
}

// connectBitbucketAppRequest is PUT /api/v1/bitbucket-app's body: the
// OAuth consumer an operator registers by hand in their Bitbucket
// workspace settings (Bitbucket, like GitLab, has no programmatic
// consumer-creation API this control plane could drive instead).
type connectBitbucketAppRequest struct {
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

// handleConnectBitbucketApp handles PUT /api/v1/bitbucket-app: saves
// the OAuth consumer's own key/secret. Does not itself obtain an access
// token; GET /api/v1/bitbucket-app/connect does that as a separate,
// real browser navigation, the same two-step "configure, then
// authorize" shape GitLab's own connect flow has.
func (rt *Router) handleConnectBitbucketApp(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the bitbucket app connection requires a master key to be configured on this control plane")
		return
	}

	var req connectBitbucketAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	req.Secret = strings.TrimSpace(req.Secret)

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if req.Secret == "" {
		writeError(w, http.StatusBadRequest, "secret is required")
		return
	}

	ctx := r.Context()
	secretsKey := store.BitbucketAppSecretsKey()
	if err := rt.bitbucketAppSecrets.SetValue(ctx, secretsKey, bitbucketAppSecretKey, req.Secret); err != nil {
		rt.internalError(w, "api: store bitbucket app secret failed", err)
		return
	}
	if err := rt.bitbucketApp.SaveBitbucketAppConnection(ctx, store.BitbucketAppConnection{
		Key:       req.Key,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		rt.internalError(w, "api: save bitbucket app connection failed", err)
		return
	}

	writeJSON(w, http.StatusOK, bitbucketAppStatusResource{
		Connected: true,
		Key:       req.Key,
	})
}

// handleDisconnectBitbucketApp handles DELETE /api/v1/bitbucket-app.
// Does not reach out to Bitbucket to revoke the token or delete the
// consumer there, the same local-only "stop trusting this connection"
// scope handleDisconnectGitHubApp/handleDisconnectGitLabApp's own doc
// comments document.
func (rt *Router) handleDisconnectBitbucketApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := rt.bitbucketApp.DeleteBitbucketAppConnection(ctx)
	if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
		writeError(w, http.StatusNotFound, "no bitbucket app is connected")
		return
	}
	if err != nil {
		rt.logger.Error("api: disconnect bitbucket app failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if rt.bitbucketAppSecrets != nil {
		if err := rt.bitbucketAppSecrets.DeleteAll(ctx, store.BitbucketAppSecretsKey()); err != nil {
			rt.logger.Error("api: delete bitbucket app secrets failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
