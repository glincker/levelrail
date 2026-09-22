package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// giteaAppRepoResource is one entry of GET /api/v1/gitea-app/repos.
type giteaAppRepoResource struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	WebURL        string `json:"web_url"`
}

func toGiteaAppRepoResource(r giteaapp.Repo) giteaAppRepoResource {
	return giteaAppRepoResource{
		FullName:      r.FullName,
		Name:          r.Name,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      r.CloneURL,
		WebURL:        r.WebURL,
	}
}

// handleListGiteaAppRepos handles GET /api/v1/gitea-app/repos: every
// repository the connected account can access, for the frontend's repo
// picker. AbilityReadSensitive, the same tier
// handleListGitLabAppProjects/handleListBitbucketAppRepos use for the
// identical reason.
func (rt *Router) handleListGiteaAppRepos(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.giteaAccessToken(ctx)
	if err != nil {
		rt.writeGiteaAppTokenError(w, "api: mint gitea access token for repo listing failed", err)
		return
	}

	repos, err := rt.giteaAppClient.ListRepos(ctx, conn.InstanceURL, accessToken)
	if err != nil {
		rt.logger.Error("api: list gitea repos failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list repositories from gitea")
		return
	}

	out := make([]giteaAppRepoResource, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toGiteaAppRepoResource(repo))
	}
	writeJSON(w, http.StatusOK, out)
}

// giteaAppBranchResource is one entry of
// GET /api/v1/gitea-app/repos/{owner}/{repo}/branches.
type giteaAppBranchResource struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

// handleListGiteaAppBranches handles
// GET /api/v1/gitea-app/repos/{owner}/{repo}/branches, the same
// AbilityReadSensitive tier and reasoning as
// handleListBitbucketAppBranches.
func (rt *Router) handleListGiteaAppBranches(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.giteaAccessToken(ctx)
	if err != nil {
		rt.writeGiteaAppTokenError(w, "api: mint gitea access token for branch listing failed", err)
		return
	}

	branches, err := rt.giteaAppClient.ListBranches(ctx, conn.InstanceURL, accessToken, owner+"/"+repo)
	if err != nil {
		rt.logger.Error("api: list gitea repo branches failed", slog.String("error", err.Error()), slog.String("owner", owner), slog.String("repo", repo))
		writeError(w, http.StatusBadGateway, "failed to list branches from gitea")
		return
	}

	out := make([]giteaAppBranchResource, 0, len(branches))
	for _, b := range branches {
		out = append(out, giteaAppBranchResource{Name: b.Name, CommitSHA: b.CommitSHA})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUseGiteaRepoAsSource handles
// POST /api/v1/gitea-app/repos/{owner}/{repo}/use-as-source. Mirrors
// handleUseBitbucketRepoAsSource, including its own documented gap: no
// deploy token is set from this path, so a private repo builds only
// once an operator pastes a token into the app's git-source card.
func (rt *Router) handleUseGiteaRepoAsSource(w http.ResponseWriter, r *http.Request) {
	if rt.giteaAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGiteaMasterKeyRequired)
		return
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}
	fullName := owner + "/" + repo

	req, buildType, triggerMode, ok := rt.decodeUseAsSourceRequest(w, r, "api: use gitea repo as source")
	if !ok {
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.giteaAccessToken(ctx)
	if err != nil {
		rt.writeGiteaAppTokenError(w, "api: mint gitea access token for use-as-source failed", err)
		return
	}

	giteaRepo, err := rt.giteaAppClient.GetRepo(ctx, conn.InstanceURL, accessToken, fullName)
	if err != nil {
		rt.logger.Error("api: get gitea repo failed", slog.String("error", err.Error()), slog.String("full_name", fullName))
		writeError(w, http.StatusBadGateway, "failed to look up the repository on gitea")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = giteaRepo.DefaultBranch
	}
	if branch == "" {
		branch = webhook.DefaultBranch
	}

	result, err := rt.connectGitSource(ctx, req.AppName, connectGitSourceParams{
		RepoURL: giteaRepo.CloneURL, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath,
		TriggerMode: triggerMode,
	})
	if err != nil {
		rt.logger.Error("api: use gitea repo as source: connect git source failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "git source connected, but set a primary domain in ingress settings before gitea can reach a webhook here")
			return
		}
		rt.logger.Error("api: get base url for gitea webhook registration failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	webhookSecret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(req.AppName), gitSourceSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve git source webhook secret for gitea hook registration failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	if err := rt.giteaAppClient.CreateRepoWebhook(ctx, conn.InstanceURL, accessToken, fullName, baseURL+result.Resource.WebhookURL, webhookSecret); err != nil {
		rt.logger.Error("api: register gitea repo webhook failed", slog.String("error", err.Error()), slog.String("full_name", fullName), slog.String("app_name", req.AppName))
		writeError(w, http.StatusBadGateway, "git source connected, but registering the webhook on gitea failed; add it manually")
		return
	}

	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: gitea repo connected as git source", slog.String("app_name", req.AppName), slog.String("full_name", fullName), slog.Bool("creating", result.Creating))
	writeJSON(w, status, result.Resource)
}
