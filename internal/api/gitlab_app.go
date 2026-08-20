package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// GitLabAppStore is the store surface the GitLab App connection handlers
// need. *store.DB satisfies this structurally, the same "core store"
// shape GitHubAppStore's own doc comment establishes.
type GitLabAppStore interface {
	GetGitLabAppConnection(ctx context.Context) (store.GitLabAppConnection, error)
	SaveGitLabAppConnection(ctx context.Context, c store.GitLabAppConnection) error
	DeleteGitLabAppConnection(ctx context.Context) error
}

// GitLabAppSecrets is the surface the GitLab App handlers need from
// internal/secrets.Manager: client_secret is set once at connect time
// and resolved back out on every OAuth token exchange/refresh and
// project API call; access_token/refresh_token/token_expires_at are
// written and re-read on the same schedule by gitlabAccessToken. The
// same "this feature both writes and reads back through
// internal/secrets" shape GitHubAppSecrets's own doc comment describes.
type GitLabAppSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// GitLabAppClient is the surface internal/api needs from
// internal/gitlabapp.Client: OAuth code exchange/refresh, and project
// listing/lookup/webhook registration once connected. *gitlabapp.Client
// satisfies this structurally; tests substitute a hand-written fake.
type GitLabAppClient interface {
	ExchangeCode(ctx context.Context, instanceURL, clientID, clientSecret, redirectURI, code string) (gitlabapp.Tokens, error)
	RefreshToken(ctx context.Context, instanceURL, clientID, clientSecret, refreshToken string) (gitlabapp.Tokens, error)
	ListProjects(ctx context.Context, instanceURL, accessToken string) ([]gitlabapp.Project, error)
	GetProject(ctx context.Context, instanceURL, accessToken string, projectID int64) (gitlabapp.Project, error)
	CreateProjectWebhook(ctx context.Context, instanceURL, accessToken string, projectID int64, hookURL, secretToken string) error
}

const (
	gitLabAppClientSecretKey = "client_secret"
	gitLabAppAccessTokenKey  = "access_token"
	gitLabAppRefreshTokenKey = "refresh_token"
	gitLabAppTokenExpiresKey = "token_expires_at"
)

// gitLabAppStatusResource is the wire shape for GET /api/v1/gitlab-app.
// No secret fields, matching gitHubAppStatusResource's own rule.
type gitLabAppStatusResource struct {
	Connected   bool   `json:"connected"`
	InstanceURL string `json:"instance_url,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	// Authorized reports whether the OAuth authorization-code flow has
	// completed (an access token exists), the GitLab counterpart of
	// gitHubAppStatusResource.Installed: a connection can be configured
	// (instance_url/client_id/client_secret saved) without yet being
	// authorized.
	Authorized bool   `json:"authorized"`
	BaseURL    string `json:"base_url,omitempty"`
}

// handleGetGitLabAppStatus handles GET /api/v1/gitlab-app.
func (rt *Router) handleGetGitLabAppStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	baseURL, _ := rt.controlPlaneBaseURL(ctx)

	conn, err := rt.gitlabApp.GetGitLabAppConnection(ctx)
	if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
		writeJSON(w, http.StatusOK, gitLabAppStatusResource{Connected: false, BaseURL: baseURL})
		return
	}
	if err != nil {
		rt.logger.Error("api: get gitlab app connection status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resource := gitLabAppStatusResource{
		Connected:   true,
		InstanceURL: conn.InstanceURL,
		ClientID:    conn.ClientID,
		CreatedAt:   conn.CreatedAt,
		BaseURL:     baseURL,
	}
	if rt.gitlabAppSecrets != nil {
		authorized, err := rt.gitlabAppSecrets.Exists(ctx, store.GitLabAppSecretsKey(), gitLabAppAccessTokenKey)
		if err != nil {
			rt.logger.Error("api: check gitlab app authorization failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resource.Authorized = authorized
	}
	writeJSON(w, http.StatusOK, resource)
}

// connectGitLabAppRequest is PUT /api/v1/gitlab-app's body: the OAuth
// Application an operator registers by hand in their GitLab instance's
// Applications settings (unlike GitHub's manifest flow, GitLab has no
// programmatic App-creation API this control plane could drive
// instead).
type connectGitLabAppRequest struct {
	InstanceURL  string `json:"instance_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// handleConnectGitLabApp handles PUT /api/v1/gitlab-app: saves the OAuth
// Application's own instance/client_id/client_secret. Does not itself
// obtain an access token; GET /api/v1/gitlab-app/connect does that as a
// separate, real browser navigation, the same two-step "configure, then
// authorize" shape GitHub App's manual-connect-then-install flow has.
func (rt *Router) handleConnectGitLabApp(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
		return
	}

	var req connectGitLabAppRequest
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
	secretsKey := store.GitLabAppSecretsKey()
	if err := rt.gitlabAppSecrets.SetValue(ctx, secretsKey, gitLabAppClientSecretKey, req.ClientSecret); err != nil {
		rt.internalError(w, "api: store gitlab app client_secret failed", err)
		return
	}
	if err := rt.gitlabApp.SaveGitLabAppConnection(ctx, store.GitLabAppConnection{
		InstanceURL: req.InstanceURL,
		ClientID:    req.ClientID,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		rt.internalError(w, "api: save gitlab app connection failed", err, slog.String("instance_url", req.InstanceURL))
		return
	}

	writeJSON(w, http.StatusOK, gitLabAppStatusResource{
		Connected:   true,
		InstanceURL: req.InstanceURL,
		ClientID:    req.ClientID,
	})
}

// handleDisconnectGitLabApp handles DELETE /api/v1/gitlab-app. Does not
// reach out to GitLab to revoke the token or delete the Application
// there, the same local-only "stop trusting this connection" scope
// handleDisconnectGitHubApp's own doc comment documents.
func (rt *Router) handleDisconnectGitLabApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := rt.gitlabApp.DeleteGitLabAppConnection(ctx)
	if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
		writeError(w, http.StatusNotFound, "no gitlab app is connected")
		return
	}
	if err != nil {
		rt.logger.Error("api: disconnect gitlab app failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if rt.gitlabAppSecrets != nil {
		if err := rt.gitlabAppSecrets.DeleteAll(ctx, store.GitLabAppSecretsKey()); err != nil {
			rt.logger.Error("api: delete gitlab app secrets failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
