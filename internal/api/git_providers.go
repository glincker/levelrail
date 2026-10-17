package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Capability summary for one git provider, enough for the git-source picker to choose its controls without a query per provider.
type gitProviderResource struct {
	Provider           string `json:"provider"`
	Connected          bool   `json:"connected"`
	CanListBranches    bool   `json:"can_list_branches"`
	CanRegisterWebhook bool   `json:"can_register_webhook"`
	CanAuthClone       bool   `json:"can_auth_clone"`
}

// Replaces three separate provider status calls. AbilityReadSensitive, since it discloses only connection state and capabilities.
func (rt *Router) handleListGitProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []gitProviderResource{
		rt.githubProviderStatus(ctx),
		rt.gitlabProviderStatus(ctx),
		rt.bitbucketProviderStatus(ctx),
		rt.giteaProviderStatus(ctx),
	}
	writeJSON(w, http.StatusOK, out)
}

// Connected means the App is connected and installed, since repo, branch, and webhook calls need the installation.
// CanAuthClone does not distinguish github.com from GitHub Enterprise Server; that clone-auth gap is tracked separately.
func (rt *Router) githubProviderStatus(ctx context.Context) gitProviderResource {
	res := gitProviderResource{Provider: "github"}
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
			rt.logger.Error("api: get github app connection for git-providers status failed", slog.String("error", err.Error()))
		}
		return res
	}
	res.Connected = conn.InstallationID != nil
	res.CanListBranches = res.Connected
	res.CanRegisterWebhook = res.Connected
	res.CanAuthClone = res.Connected
	return res
}

// Connected means OAuth is authorized. CanAuthClone stays false: GitLab has no authenticated clone path today.
func (rt *Router) gitlabProviderStatus(ctx context.Context) gitProviderResource {
	res := gitProviderResource{Provider: "gitlab"}
	if _, err := rt.gitlabApp.GetGitLabAppConnection(ctx); err != nil {
		if !errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
			rt.logger.Error("api: get gitlab app connection for git-providers status failed", slog.String("error", err.Error()))
		}
		return res
	}
	if rt.gitlabAppSecrets == nil {
		return res
	}
	authorized, err := rt.gitlabAppSecrets.Exists(ctx, store.GitLabAppSecretsKey(), gitLabAppAccessTokenKey)
	if err != nil {
		rt.logger.Error("api: check gitlab app authorization for git-providers status failed", slog.String("error", err.Error()))
		return res
	}
	res.Connected = authorized
	res.CanListBranches = authorized
	res.CanRegisterWebhook = authorized
	return res
}

// Connected means authorized. CanAuthClone stays false: Bitbucket has no authenticated clone path today.
func (rt *Router) bitbucketProviderStatus(ctx context.Context) gitProviderResource {
	res := gitProviderResource{Provider: "bitbucket"}
	if _, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx); err != nil {
		if !errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
			rt.logger.Error("api: get bitbucket app connection for git-providers status failed", slog.String("error", err.Error()))
		}
		return res
	}
	if rt.bitbucketAppSecrets == nil {
		return res
	}
	authorized, err := rt.bitbucketAppSecrets.Exists(ctx, store.BitbucketAppSecretsKey(), bitbucketAppAccessTokenKey)
	if err != nil {
		rt.logger.Error("api: check bitbucket app authorization for git-providers status failed", slog.String("error", err.Error()))
		return res
	}
	res.Connected = authorized
	res.CanListBranches = authorized
	res.CanRegisterWebhook = authorized
	return res
}

// Connected means authorized. CanAuthClone stays false: Gitea has no authenticated clone path today.
func (rt *Router) giteaProviderStatus(ctx context.Context) gitProviderResource {
	res := gitProviderResource{Provider: "gitea"}
	if _, err := rt.giteaApp.GetGiteaAppConnection(ctx); err != nil {
		if !errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
			rt.logger.Error("api: get gitea app connection for git-providers status failed", slog.String("error", err.Error()))
		}
		return res
	}
	if rt.giteaAppSecrets == nil {
		return res
	}
	authorized, err := rt.giteaAppSecrets.Exists(ctx, store.GiteaAppSecretsKey(), giteaAppAccessTokenKey)
	if err != nil {
		rt.logger.Error("api: check gitea app authorization for git-providers status failed", slog.String("error", err.Error()))
		return res
	}
	res.Connected = authorized
	res.CanListBranches = authorized
	res.CanRegisterWebhook = authorized
	return res
}
