package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// manualGitHubAppRequest is PUT /api/v1/github-app/manual's request
// body: every field a GitHub App's own "General" settings page shows an
// operator who created the App by hand (github.com/settings/apps/new,
// no manifest), the same field set connect.go's manifest-flow callback
// receives from GitHub's own API response, just typed in instead of
// exchanged automatically. InstallationID/AccountLogin are optional:
// an operator who hasn't installed the App on a repo yet can save the
// App-level credentials now and install later via the existing
// GET /api/v1/github-app/installed redirect, the same two-step shape
// the manifest flow already has.
type manualGitHubAppRequest struct {
	AppID          int64   `json:"app_id"`
	ClientID       string  `json:"client_id"`
	ClientSecret   string  `json:"client_secret"`
	WebhookSecret  string  `json:"webhook_secret"`
	PrivateKeyPEM  string  `json:"private_key"`
	InstallationID *int64  `json:"installation_id,omitempty"`
	AccountLogin   *string `json:"account_login,omitempty"`
}

// handleConnectGitHubAppManually handles PUT /api/v1/github-app/manual:
// the alternative to the automated manifest flow
// (handleStartGitHubAppRegistration/handleGitHubAppCallback) for an
// operator whose control plane has no publicly reachable primary domain
// yet (controlPlaneBaseURL's own error), or who simply prefers creating
// the App by hand on github.com and pasting the resulting credentials
// in, the same "automated vs manual" choice Coolify's own GitHub App
// connect screen offers side by side. Storage is identical to the
// manifest flow's own handleGitHubAppCallback: same three secrets keys,
// same SaveGitHubAppConnection row, so both paths produce an
// indistinguishable connected state once saved.
func (rt *Router) handleConnectGitHubAppManually(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	var req manualGitHubAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)
	req.WebhookSecret = strings.TrimSpace(req.WebhookSecret)
	req.PrivateKeyPEM = strings.TrimSpace(req.PrivateKeyPEM)

	if req.AppID <= 0 {
		writeError(w, http.StatusBadRequest, "app_id is required")
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
	if req.WebhookSecret == "" {
		writeError(w, http.StatusBadRequest, "webhook_secret is required")
		return
	}
	if req.PrivateKeyPEM == "" {
		writeError(w, http.StatusBadRequest, "private_key is required")
		return
	}
	if err := githubapp.ValidatePrivateKeyPEM([]byte(req.PrivateKeyPEM)); err != nil {
		writeError(w, http.StatusBadRequest, "private_key is not a valid RSA private key: "+err.Error())
		return
	}

	ctx := r.Context()
	secretsKey := store.GitHubAppSecretsKey()
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "client_secret", req.ClientSecret); err != nil {
		rt.internalError(w, "api: store github app client_secret failed", err)
		return
	}
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "webhook_secret", req.WebhookSecret); err != nil {
		rt.internalError(w, "api: store github app webhook_secret failed", err)
		return
	}
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "private_key", req.PrivateKeyPEM); err != nil {
		rt.internalError(w, "api: store github app private_key failed", err)
		return
	}

	if err := rt.githubApp.SaveGitHubAppConnection(ctx, store.GitHubAppConnection{
		AppID:          req.AppID,
		ClientID:       req.ClientID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		InstallationID: req.InstallationID,
		AccountLogin:   req.AccountLogin,
	}); err != nil {
		rt.internalError(w, "api: save github app connection failed", err, slog.Int64("app_id", req.AppID))
		return
	}

	writeJSON(w, http.StatusOK, gitHubAppStatusResource{
		Connected: true,
		AppID:     req.AppID,
		ClientID:  req.ClientID,
	})
}
