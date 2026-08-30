package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// gitProviderResource is one entry of GET /api/v1/git-providers: a
// capability summary for one provider, enough for the git-source picker
// to decide whether to show a connected repo/branch picker or an
// unconnected "Connect" row, and whether to render a branch select or a
// free-text fallback, without a suspense query per provider (see this
// file's own doc comment on handleListGitProviders for why that
// mattered).
type gitProviderResource struct {
	Provider           string `json:"provider"`
	Connected          bool   `json:"connected"`
	CanListBranches    bool   `json:"can_list_branches"`
	CanRegisterWebhook bool   `json:"can_register_webhook"`
	CanAuthClone       bool   `json:"can_auth_clone"`
}

// handleListGitProviders handles GET /api/v1/git-providers: one call
// replacing the three separate GET .../github-app, .../gitlab-app,
// .../bitbucket-app status calls the git-source picker would otherwise
// need. AbilityReadSensitive, not AbilityRoot like those three status
// endpoints themselves: this discloses nothing beyond "is provider X
// connected and what can it do", the same disclosure level GET .../repos
// already sits at for each provider individually (see
// handleListGitHubAppRepos's own doc comment). Before this endpoint
// existed, a non-root deploy-scoped user opening the app creation wizard
// threw into an error boundary, because the wizard's useSuspenseQuery
// hooks called those three AbilityRoot endpoints directly.
func (rt *Router) handleListGitProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []gitProviderResource{
		rt.githubProviderStatus(ctx),
		rt.gitlabProviderStatus(ctx),
		rt.bitbucketProviderStatus(ctx),
	}
	writeJSON(w, http.StatusOK, out)
}

// githubProviderStatus reports GitHub's entry. Connected means the App
// is both connected and installed (mintGitHubAppInstallationToken's own
// precondition for every repo/branch/webhook call): a connection row
// with no installation yet can't list repos or register a webhook, so
// it isn't "connected" from the picker's point of view either.
// CanAuthClone doesn't distinguish github.com from a GitHub Enterprise
// Server instance: the GHE clone-auth gap (isGitHubHTTPSRepoURL
// hardcoding github.com, docs-local/research/git-provider-connect-ux-
// unification-proposal.md section 1) is a separately tracked bug, not
// this endpoint's concern to model.
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

// gitlabProviderStatus reports GitLab's entry. Connected means the
// OAuth Application is authorized (an access token exists): a
// configured-but-not-yet-authorized connection can't list projects
// either. CanAuthClone stays false: GitLab has no authenticated clone
// path today (docs-local/research/git-provider-connect-ux-unification-
// proposal.md section 3's capability table).
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

// bitbucketProviderStatus reports Bitbucket's entry, the same
// "connected means authorized" shape gitlabProviderStatus uses.
// CanAuthClone stays false, matching Bitbucket's own capability table
// entry (no authenticated clone path today).
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
