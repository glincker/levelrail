package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// gitLabAppProjectResource is one entry of GET /api/v1/gitlab-app/projects.
type gitLabAppProjectResource struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	CloneURL          string `json:"clone_url"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
}

func toGitLabAppProjectResource(p gitlabapp.Project) gitLabAppProjectResource {
	return gitLabAppProjectResource{
		ID:                p.ID,
		Name:              p.Name,
		PathWithNamespace: p.PathWithNamespace,
		CloneURL:          p.HTTPURLToRepo,
		DefaultBranch:     p.DefaultBranch,
		Visibility:        p.Visibility,
		WebURL:            p.WebURL,
	}
}

// handleListGitLabAppProjects handles GET /api/v1/gitlab-app/projects:
// every project the connected account can access, for the frontend's
// project picker. AbilityReadSensitive, the same tier
// handleListGitHubAppRepos uses for the identical reason (this discloses
// real private-project names/paths).
func (rt *Router) handleListGitLabAppProjects(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.gitlabAccessToken(ctx)
	if err != nil {
		rt.writeGitLabAppTokenError(w, "api: mint gitlab access token for project listing failed", err)
		return
	}

	projects, err := rt.gitlabAppClient.ListProjects(ctx, conn.InstanceURL, accessToken)
	if err != nil {
		rt.logger.Error("api: list gitlab projects failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list projects from gitlab")
		return
	}

	out := make([]gitLabAppProjectResource, 0, len(projects))
	for _, p := range projects {
		out = append(out, toGitLabAppProjectResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// gitLabAppBranchResource is one entry of
// GET /api/v1/gitlab-app/projects/{id}/branches.
type gitLabAppBranchResource struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

// handleListGitLabAppBranches handles
// GET /api/v1/gitlab-app/projects/{id}/branches, the same
// AbilityReadSensitive tier and reasoning as handleListGitLabAppProjects.
func (rt *Router) handleListGitLabAppBranches(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
		return
	}

	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.gitlabAccessToken(ctx)
	if err != nil {
		rt.writeGitLabAppTokenError(w, "api: mint gitlab access token for branch listing failed", err)
		return
	}

	branches, err := rt.gitlabAppClient.ListBranches(ctx, conn.InstanceURL, accessToken, projectID)
	if err != nil {
		rt.logger.Error("api: list gitlab project branches failed", slog.String("error", err.Error()), slog.Int64("project_id", projectID))
		writeError(w, http.StatusBadGateway, "failed to list branches from gitlab")
		return
	}

	out := make([]gitLabAppBranchResource, 0, len(branches))
	for _, b := range branches {
		out = append(out, gitLabAppBranchResource{Name: b.Name, CommitSHA: b.CommitSHA})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUseGitLabProjectAsSource handles
// POST /api/v1/gitlab-app/projects/{id}/use-as-source.
//
// Known gap: no deploy token is set from this path, so a private
// project builds only once an operator pastes a token into the app's
// git-source card; the short-lived OAuth token isn't reused for that.
func (rt *Router) handleUseGitLabProjectAsSource(w http.ResponseWriter, r *http.Request) {
	if rt.gitlabAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the gitlab app connection requires a master key to be configured on this control plane")
		return
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	req, buildType, ok := rt.decodeUseAsSourceRequest(w, r, "api: use gitlab project as source")
	if !ok {
		return
	}

	ctx := r.Context()
	conn, accessToken, err := rt.gitlabAccessToken(ctx)
	if err != nil {
		rt.writeGitLabAppTokenError(w, "api: mint gitlab access token for use-as-source failed", err)
		return
	}

	project, err := rt.gitlabAppClient.GetProject(ctx, conn.InstanceURL, accessToken, projectID)
	if err != nil {
		rt.logger.Error("api: get gitlab project failed", slog.String("error", err.Error()), slog.Int64("project_id", projectID))
		writeError(w, http.StatusBadGateway, "failed to look up the project on gitlab")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = project.DefaultBranch
	}
	if branch == "" {
		branch = webhook.DefaultBranch
	}

	result, err := rt.connectGitSource(ctx, req.AppName, connectGitSourceParams{
		RepoURL: project.HTTPURLToRepo, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath,
	})
	if err != nil {
		rt.logger.Error("api: use gitlab project as source: connect git source failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "git source connected, but set a primary domain in ingress settings before gitlab can reach a webhook here")
			return
		}
		rt.logger.Error("api: get base url for gitlab webhook registration failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	webhookSecret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(req.AppName), gitSourceSecretKey)
	if err != nil {
		rt.logger.Error("api: resolve git source webhook secret for gitlab hook registration failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.gitlabAppClient.CreateProjectWebhook(ctx, conn.InstanceURL, accessToken, projectID, baseURL+result.Resource.WebhookURL, webhookSecret); err != nil {
		rt.logger.Error("api: register gitlab project webhook failed", slog.String("error", err.Error()), slog.Int64("project_id", projectID), slog.String("app_name", req.AppName))
		writeError(w, http.StatusBadGateway, "git source connected, but registering the webhook on gitlab failed; add it manually")
		return
	}

	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: gitlab project connected as git source", slog.String("app_name", req.AppName), slog.Int64("project_id", projectID), slog.Bool("creating", result.Creating))
	writeJSON(w, status, result.Resource)
}
