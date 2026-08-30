package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// bitbucketAppRepoResource is one entry of GET /api/v1/bitbucket-app/repos.
type bitbucketAppRepoResource struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	WebURL        string `json:"web_url"`
}

func toBitbucketAppRepoResource(r bitbucketapp.Repo) bitbucketAppRepoResource {
	return bitbucketAppRepoResource{
		FullName:      r.FullName,
		Name:          r.Name,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      r.CloneURL,
		WebURL:        r.WebURL,
	}
}

// handleListBitbucketAppRepos handles GET /api/v1/bitbucket-app/repos:
// every repository the connected account can access, for the
// frontend's repo picker. AbilityReadSensitive, the same tier
// handleListGitHubAppRepos/handleListGitLabAppProjects use for the
// identical reason.
func (rt *Router) handleListBitbucketAppRepos(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errBitbucketMasterKeyRequired)
		return
	}

	ctx := r.Context()
	accessToken, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		rt.writeBitbucketAppTokenError(w, "api: mint bitbucket access token for repo listing failed", err)
		return
	}

	repos, err := rt.bitbucketAppClient.ListRepos(ctx, accessToken)
	if err != nil {
		rt.logger.Error("api: list bitbucket repos failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list repositories from bitbucket")
		return
	}

	out := make([]bitbucketAppRepoResource, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toBitbucketAppRepoResource(repo))
	}
	writeJSON(w, http.StatusOK, out)
}

// bitbucketAppBranchResource is one entry of
// GET /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches.
type bitbucketAppBranchResource struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

// handleListBitbucketAppBranches handles
// GET /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches, the
// same AbilityReadSensitive tier and reasoning as
// handleListGitHubAppBranches.
func (rt *Router) handleListBitbucketAppBranches(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errBitbucketMasterKeyRequired)
		return
	}

	workspace := r.PathValue("workspace")
	repoSlug := r.PathValue("repoSlug")
	if workspace == "" || repoSlug == "" {
		writeError(w, http.StatusBadRequest, "workspace and repoSlug are required")
		return
	}

	ctx := r.Context()
	accessToken, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		rt.writeBitbucketAppTokenError(w, "api: mint bitbucket access token for branch listing failed", err)
		return
	}

	branches, err := rt.bitbucketAppClient.ListBranches(ctx, accessToken, workspace+"/"+repoSlug)
	if err != nil {
		rt.logger.Error("api: list bitbucket repo branches failed", slog.String("error", err.Error()), slog.String("workspace", workspace), slog.String("repo_slug", repoSlug))
		writeError(w, http.StatusBadGateway, "failed to list branches from bitbucket")
		return
	}

	out := make([]bitbucketAppBranchResource, 0, len(branches))
	for _, b := range branches {
		out = append(out, bitbucketAppBranchResource{Name: b.Name, CommitSHA: b.CommitSHA})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUseBitbucketRepoAsSource handles
// POST /api/v1/bitbucket-app/repos/{fullName}/use-as-source. Mirrors
// handleUseGitLabProjectAsSource, including its own documented gap: no
// deploy token is set from this path, so a private repo builds only
// once an operator pastes a token into the app's git-source card.
func (rt *Router) handleUseBitbucketRepoAsSource(w http.ResponseWriter, r *http.Request) {
	if rt.bitbucketAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errBitbucketMasterKeyRequired)
		return
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	workspace := r.PathValue("workspace")
	repoSlug := r.PathValue("repoSlug")
	if workspace == "" || repoSlug == "" {
		writeError(w, http.StatusBadRequest, "workspace and repoSlug are required")
		return
	}
	fullName := workspace + "/" + repoSlug

	req, buildType, ok := rt.decodeUseAsSourceRequest(w, r, "api: use bitbucket repo as source")
	if !ok {
		return
	}

	ctx := r.Context()
	accessToken, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		rt.writeBitbucketAppTokenError(w, "api: mint bitbucket access token for use-as-source failed", err)
		return
	}

	repo, err := rt.bitbucketAppClient.GetRepo(ctx, accessToken, fullName)
	if err != nil {
		rt.logger.Error("api: get bitbucket repo failed", slog.String("error", err.Error()), slog.String("full_name", fullName))
		writeError(w, http.StatusBadGateway, "failed to look up the repository on bitbucket")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if branch == "" {
		branch = webhook.DefaultBranch
	}

	result, err := rt.connectGitSource(ctx, req.AppName, connectGitSourceParams{
		RepoURL: repo.CloneURL, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath,
	})
	if err != nil {
		rt.logger.Error("api: use bitbucket repo as source: connect git source failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "git source connected, but set a primary domain in ingress settings before bitbucket can reach a webhook here")
			return
		}
		rt.logger.Error("api: get base url for bitbucket webhook registration failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	webhookSecret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(req.AppName), gitSourceSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve git source webhook secret for bitbucket hook registration failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	if err := rt.bitbucketAppClient.CreateRepoWebhook(ctx, accessToken, fullName, baseURL+result.Resource.WebhookURL, webhookSecret); err != nil {
		rt.logger.Error("api: register bitbucket repo webhook failed", slog.String("error", err.Error()), slog.String("full_name", fullName), slog.String("app_name", req.AppName))
		writeError(w, http.StatusBadGateway, "git source connected, but registering the webhook on bitbucket failed; add it manually")
		return
	}

	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: bitbucket repo connected as git source", slog.String("app_name", req.AppName), slog.String("full_name", fullName), slog.Bool("creating", result.Creating))
	writeJSON(w, status, result.Resource)
}
