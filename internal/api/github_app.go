package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// GitHubAppStore is the store surface the GitHub App connection
// handlers need. *store.DB satisfies this structurally, part of the
// core Store interface (github_app_connections always exists as a
// table even when empty, the same "core store, not an optional
// plug-in" shape ingressSettings/domains already establish).
type GitHubAppStore interface {
	GetGitHubAppConnection(ctx context.Context) (store.GitHubAppConnection, error)
	SaveGitHubAppConnection(ctx context.Context, c store.GitHubAppConnection) error
	UpdateGitHubAppInstallation(ctx context.Context, installationID int64, accountLogin string) error
	DeleteGitHubAppConnection(ctx context.Context) error
}

// GitHubAppSecrets is the surface the GitHub App handlers need from
// internal/secrets.Manager: both SetValue (storing client_secret/
// webhook_secret/private_key right after a successful code exchange)
// and Resolve (decrypting the private key back out, in memory only,
// immediately before signing a JWT). A different shape from
// BackupSecretsSetter (set-only) and internal/backup.SecretsResolver
// (resolve-only): this is the one credential-bearing feature in this
// codebase that both writes and reads back through internal/secrets
// from inside internal/api itself, because minting a fresh installation
// token on every repo/branch-listing call (see github_app_repos.go)
// needs the private key synchronously in the same request, not on a
// separate worker path the way internal/backup.Runner's own Resolve
// calls are. *secrets.Manager satisfies this structurally.
type GitHubAppSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// GitHubAppClient is the surface internal/api needs from
// internal/githubapp.Client: manifest code exchange, installation
// lookup and token minting, repo/branch listing, and repo webhook
// registration. *githubapp.Client satisfies this structurally; tests
// substitute a hand-written fake (github_app_test.go), the same
// "narrow, consumer-defined interface" shape every other external-system
// boundary in this package uses (Builder, DockerPinger, BackupRunner).
type GitHubAppClient interface {
	CheckInstanceReachable(ctx context.Context, instanceURL string) error
	ExchangeManifestCode(ctx context.Context, instanceURL, code string) (githubapp.Credentials, error)
	GetInstallation(ctx context.Context, instanceURL, appJWT string, installationID int64) (githubapp.InstallationInfo, error)
	MintInstallationToken(ctx context.Context, instanceURL, appJWT string, installationID int64) (githubapp.InstallationToken, error)
	ListInstallationRepos(ctx context.Context, instanceURL, token string) ([]githubapp.Repo, error)
	GetRepo(ctx context.Context, instanceURL, token, owner, repo string) (githubapp.Repo, error)
	ListBranches(ctx context.Context, instanceURL, token, owner, repo string) ([]githubapp.Branch, error)
	CreateRepoWebhook(ctx context.Context, instanceURL, token, owner, repo, hookURL, secret string) error
}

// gitHubAppStatusResource is the wire shape for GET /api/v1/github-app.
// Deliberately no secret fields (client_secret/webhook_secret/
// private_key never leave internal/secrets through this or any other
// response body): AppID and ClientID are the only identity fields
// echoed back, both non-secret per this feature's own store design
// (migrations/0034_github_app_connection.sql's own comment).
type gitHubAppStatusResource struct {
	Connected bool   `json:"connected"`
	AppID     int64  `json:"app_id,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	// InstanceURL is "https://github.com" for every connection until
	// GitHub Enterprise Server support (migrations/0061), a real GHES
	// base URL after. Empty when not connected, the same shape
	// GitLabAppStatus's own instance_url already has.
	InstanceURL  string `json:"instance_url,omitempty"`
	Installed    bool   `json:"installed"`
	AccountLogin string `json:"account_login,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	// BaseURL is controlPlaneBaseURL's own result, empty when no primary
	// domain is configured yet. Surfaced here so the frontend can show
	// exactly what host the automated flow's callback/webhook URLs will
	// point at before the operator clicks through to GitHub, rather than
	// discovering a domain mismatch only after GitHub redirects back to
	// an address that isn't this running instance.
	BaseURL string `json:"base_url,omitempty"`
}

// handleGetGitHubAppStatus handles GET /api/v1/github-app: the
// connection status the settings page renders (not connected /
// connected as <account> / connected but not yet installed).
// AbilityRoot: see router.go's own route registration comment for why
// every GitHub App connection route, including this read, shares the
// same tier as PUT /api/v1/settings/ingress rather than the plain
// AbilityRead visibility tier most other GET routes use.
func (rt *Router) handleGetGitHubAppStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Ignored error: an unconfigured primary domain is a real, common
	// state (surfaced to the frontend as an empty BaseURL, not a 500),
	// not a store failure worth logging.
	baseURL, _ := rt.controlPlaneBaseURL(ctx)

	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		writeJSON(w, http.StatusOK, gitHubAppStatusResource{Connected: false, BaseURL: baseURL})
		return
	}
	if err != nil {
		rt.logger.Error("api: get github app connection status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resource := gitHubAppStatusResource{
		Connected:   true,
		AppID:       conn.AppID,
		ClientID:    conn.ClientID,
		InstanceURL: conn.InstanceURL,
		CreatedAt:   conn.CreatedAt,
		BaseURL:     baseURL,
	}
	if conn.InstallationID != nil {
		resource.Installed = true
	}
	if conn.AccountLogin != nil {
		resource.AccountLogin = *conn.AccountLogin
	}
	writeJSON(w, http.StatusOK, resource)
}

// handleDisconnectGitHubApp handles DELETE /api/v1/github-app. See
// store.DB.DeleteGitHubAppConnection's own doc comment for the known,
// accepted gap this shares with DeleteBackupTarget: the App's own
// secret values (client_secret/webhook_secret/private_key) are not
// erased from internal/secrets, only the store row that made them
// reachable through this feature. Does not call GitHub to uninstall or
// delete the App on GitHub's own side: this is a local
// "stop trusting this connection" action, not a remote teardown. An
// operator who wants the App itself gone from GitHub must delete it
// from github.com/settings/apps themselves.
func (rt *Router) handleDisconnectGitHubApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := rt.githubApp.DeleteGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		writeError(w, http.StatusNotFound, "no github app is connected")
		return
	}
	if err != nil {
		rt.logger.Error("api: disconnect github app failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// The connection row is gone; the App's client_secret/webhook_secret/
	// private_key must go with it, not linger decryptable under the same
	// sentinel key indefinitely (see DeleteAll's own doc comment). A real
	// connection row is only ever created by handleGitHubAppCallback,
	// itself gated on rt.githubAppSecrets != nil, but this checks again
	// rather than assuming: a row can also reach the store directly
	// (tests, a future migration/import path) without that invariant
	// holding.
	if rt.githubAppSecrets != nil {
		if err := rt.githubAppSecrets.DeleteAll(ctx, store.GitHubAppSecretsKey()); err != nil {
			rt.logger.Error("api: delete github app secrets failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
