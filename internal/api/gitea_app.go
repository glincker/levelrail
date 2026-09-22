package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// GiteaAppStore is the store surface the Gitea App connection handlers
// need. *store.DB satisfies this structurally, the same "core store"
// shape GitLabAppStore's own doc comment establishes.
type GiteaAppStore interface {
	GetGiteaAppConnection(ctx context.Context) (store.GiteaAppConnection, error)
	SaveGiteaAppConnection(ctx context.Context, c store.GiteaAppConnection) error
	DeleteGiteaAppConnection(ctx context.Context) error
}

// GiteaAppSecrets is the surface the Gitea App handlers need from
// internal/secrets.Manager: client_secret is set once at connect time
// and resolved back out on every OAuth token exchange/refresh and
// repo/branch API call; access_token/refresh_token/token_expires_at are
// written and re-read on the same schedule by giteaAccessToken. The
// same "this feature both writes and reads back through
// internal/secrets" shape GitLabAppSecrets's own doc comment describes.
type GiteaAppSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// GiteaAppClient is the surface internal/api needs from
// internal/giteaapp.Client: OAuth code exchange/refresh, and repo
// listing/lookup/webhook registration once connected. *giteaapp.Client
// satisfies this structurally; tests substitute a hand-written fake.
type GiteaAppClient interface {
	ExchangeCode(ctx context.Context, instanceURL, clientID, clientSecret, redirectURI, code string) (giteaapp.Tokens, error)
	RefreshToken(ctx context.Context, instanceURL, clientID, clientSecret, refreshToken string) (giteaapp.Tokens, error)
	ListRepos(ctx context.Context, instanceURL, accessToken string) ([]giteaapp.Repo, error)
	GetRepo(ctx context.Context, instanceURL, accessToken, fullName string) (giteaapp.Repo, error)
	ListBranches(ctx context.Context, instanceURL, accessToken, fullName string) ([]giteaapp.Branch, error)
	CreateRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName, hookURL, secret string) error
}

const (
	giteaAppClientSecretKey = "client_secret"
	giteaAppAccessTokenKey  = "access_token"
	giteaAppRefreshTokenKey = "refresh_token"
	giteaAppTokenExpiresKey = "token_expires_at"

	errGiteaMasterKeyRequired = "the gitea app connection requires a master key to be configured on this control plane"
)

// giteaAppStatusResource is the wire shape for GET /api/v1/gitea-app. No
// secret fields, matching gitLabAppStatusResource's own rule.
type giteaAppStatusResource struct {
	Connected   bool   `json:"connected"`
	InstanceURL string `json:"instance_url,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	// Authorized reports whether the OAuth authorization-code flow has
	// completed (an access token exists), the same "configured but not
	// yet authorized" distinction gitLabAppStatusResource.Authorized's
	// own doc comment makes.
	Authorized bool   `json:"authorized"`
	BaseURL    string `json:"base_url,omitempty"`
}

// handleGetGiteaAppStatus handles GET /api/v1/gitea-app.
func (rt *Router) handleGetGiteaAppStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	baseURL, _ := rt.controlPlaneBaseURL(ctx)

	conn, err := rt.giteaApp.GetGiteaAppConnection(ctx)
	if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
		writeJSON(w, http.StatusOK, giteaAppStatusResource{Connected: false, BaseURL: baseURL})
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitea app connection status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	resource := giteaAppStatusResource{
		Connected:   true,
		InstanceURL: conn.InstanceURL,
		ClientID:    conn.ClientID,
		CreatedAt:   conn.CreatedAt,
		BaseURL:     baseURL,
	}
	if rt.giteaAppSecrets != nil {
		authorized, err := rt.giteaAppSecrets.Exists(ctx, store.GiteaAppSecretsKey(), giteaAppAccessTokenKey)
		if err != nil {
			rt.logger.Error("api: check gitea app authorization failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, errInternal)
			return
		}
		resource.Authorized = authorized
	}
	writeJSON(w, http.StatusOK, resource)
}

// connectGiteaAppRequest is PUT /api/v1/gitea-app's body: the OAuth
// Application an operator registers by hand in their Gitea instance's
// Applications settings (Gitea, like GitLab, has no programmatic
// App-creation API this control plane could drive instead).
type connectGiteaAppRequest struct {
	InstanceURL  string `json:"instance_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// handleConnectGiteaApp handles PUT /api/v1/gitea-app: saves the OAuth
// Application's own instance/client_id/client_secret. Does not itself
// obtain an access token; GET /api/v1/gitea-app/connect does that as a
// separate, real browser navigation, the same two-step "configure, then
// authorize" shape GitLab App's own connect flow has.
func (rt *Router) handleConnectGiteaApp(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}

	var req connectGiteaAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.InstanceURL = strings.TrimRight(strings.TrimSpace(req.InstanceURL), "/")
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)

	if req.InstanceURL == "" {
		writeError(w, http.StatusBadRequest, "instance_url is required")
		return
	}
	if err := requireHTTPOrHTTPSScheme(req.InstanceURL); err != nil {
		writeError(w, http.StatusBadRequest, "instance_url must use http or https")
		return
	}
	if req.ClientID == "" {
		writeError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "client_secret is required")
		return
	}

	ctx := r.Context()
	secretsKey := store.GiteaAppSecretsKey()
	if err := rt.giteaAppSecrets.SetValue(ctx, secretsKey, giteaAppClientSecretKey, req.ClientSecret); err != nil {
		rt.internalError(w, "api: store gitea app client_secret failed", err)
		return
	}
	if err := rt.giteaApp.SaveGiteaAppConnection(ctx, store.GiteaAppConnection{
		InstanceURL: req.InstanceURL,
		ClientID:    req.ClientID,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		rt.internalError(w, "api: save gitea app connection failed", err, slog.String("instance_url", req.InstanceURL))
		return
	}

	writeJSON(w, http.StatusOK, giteaAppStatusResource{
		Connected:   true,
		InstanceURL: req.InstanceURL,
		ClientID:    req.ClientID,
	})
}

// handleDisconnectGiteaApp handles DELETE /api/v1/gitea-app. Does not
// reach out to Gitea to revoke the token or delete the Application
// there, the same local-only "stop trusting this connection" scope
// handleDisconnectGitLabApp's own doc comment documents.
func (rt *Router) handleDisconnectGiteaApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := rt.giteaApp.DeleteGiteaAppConnection(ctx)
	if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
		writeError(w, http.StatusNotFound, "no gitea app is connected")
		return
	}
	if err != nil {
		rt.logger.Error("api: disconnect gitea app failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	if rt.giteaAppSecrets != nil {
		if err := rt.giteaAppSecrets.DeleteAll(ctx, store.GiteaAppSecretsKey()); err != nil {
			rt.logger.Error("api: delete gitea app secrets failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, errInternal)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
