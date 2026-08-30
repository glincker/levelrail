package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// useGitHubRepoAsSourceResponse is gitSourceResource extended with
// whether the push webhook was actually auto-registered. Unlike
// handleUseGitLabProjectAsSource/handleUseBitbucketRepoAsSource (which
// 502 the whole request on any webhook registration failure), a
// permission-denied failure here (see githubapp.ErrPermissionDenied)
// still connects the source and reports WebhookRegistered: false plus
// WebhookError instead, because that failure is structural and
// permanent for an installation that predates repository_hooks:write,
// not a transient error worth blocking the connect on. Any other
// failure still 502s, same as GitLab/Bitbucket.
type useGitHubRepoAsSourceResponse struct {
	gitSourceResource
	WebhookRegistered bool   `json:"webhook_registered"`
	WebhookError      string `json:"webhook_error,omitempty"`
}

// handleUseGitHubRepoAsSource handles
// POST /api/v1/github-app/repos/{owner}/{repo}/use-as-source: connects
// the repo as appName's git source and registers a push webhook with
// the installation token, mirroring handleUseGitLabProjectAsSource's
// and handleUseBitbucketRepoAsSource's shape. See
// useGitHubRepoAsSourceResponse's own doc comment for the one real
// difference, the permission-denied degrade path.
func (rt *Router) handleUseGitHubRepoAsSource(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
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

	req, buildType, ok := rt.decodeUseAsSourceRequest(w, r, "api: use github repo as source")
	if !ok {
		return
	}

	ctx := r.Context()
	instanceURL, token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.writeGitHubAppTokenError(w, "api: mint github app installation token for use-as-source failed", err)
		return
	}

	repoInfo, err := rt.githubAppClient.GetRepo(ctx, instanceURL, token, owner, repo)
	if err != nil {
		rt.logger.Error("api: get github repo failed", slog.String("error", err.Error()), slog.String("owner", owner), slog.String("repo", repo))
		writeError(w, http.StatusBadGateway, "failed to look up the repository on github")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = repoInfo.DefaultBranch
	}
	if branch == "" {
		branch = webhook.DefaultBranch
	}

	result, err := rt.connectGitSource(ctx, req.AppName, connectGitSourceParams{
		RepoURL: instanceURL + "/" + repoInfo.FullName + ".git", Branch: branch, BuildType: buildType, BuildPath: req.BuildPath,
	})
	if err != nil {
		rt.logger.Error("api: use github repo as source: connect git source failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	resp := useGitHubRepoAsSourceResponse{gitSourceResource: result.Resource}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if !errors.Is(err, errNoPrimaryDomain) {
			rt.logger.Error("api: get base url for github webhook registration failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: github repo connected as git source, but no primary domain is configured to register a webhook", slog.String("app_name", req.AppName))
		resp.WebhookError = "set a primary domain in ingress settings, then register this webhook manually: no reachable url exists yet"
		writeJSON(w, status, resp)
		return
	}

	webhookSecret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(req.AppName), gitSourceSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve git source webhook secret for github hook registration failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.githubAppClient.CreateRepoWebhook(ctx, instanceURL, token, owner, repo, baseURL+result.Resource.WebhookURL, webhookSecret); err != nil {
		if errors.Is(err, githubapp.ErrPermissionDenied) {
			rt.logger.Warn("api: github repo connected as git source, but webhook registration was denied by the installation's permissions", slog.String("owner", owner), slog.String("repo", repo), slog.String("app_name", req.AppName))
			resp.WebhookError = "this github app installation doesn't have permission to register webhooks yet (it predates that permission being requested); add the webhook below manually, or reinstall the app to grant it and reconnect"
			writeJSON(w, status, resp)
			return
		}
		rt.logger.Error("api: register github repo webhook failed", slog.String("error", err.Error()), slog.String("owner", owner), slog.String("repo", repo), slog.String("app_name", req.AppName))
		writeError(w, http.StatusBadGateway, "git source connected, but registering the webhook on github failed; add it manually")
		return
	}

	resp.WebhookRegistered = true
	rt.logger.Info("api: github repo connected as git source", slog.String("app_name", req.AppName), slog.String("owner", owner), slog.String("repo", repo), slog.Bool("creating", result.Creating))
	writeJSON(w, status, resp)
}
