package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gitLabStatusDescriptionMax is GitLab's own documented limit for a
// commit status's description field.
const gitLabStatusDescriptionMax = 255

// previewGitLabTarget resolves what CreateMergeRequestNote/
// CreateCommitStatus need to notify GitLab about gs's preview: an OAuth
// access token and gs's own URL-encoded project path. ok is false, and
// every caller here treats that as a silent no-op, whenever
// gs.PostPRComments is off, no GitLab OAuth application is connected and
// authorized, or gs.RepoURL isn't actually hosted on the connected
// instance: posting a comment or status is cosmetic to the preview it
// annotates, the same "logged, not fatal" shape previewGitHubTarget
// already establishes.
func (rt *Router) previewGitLabTarget(ctx context.Context, appName string, gs store.GitSource) (instanceURL, accessToken, projectPath string, ok bool) {
	if !gs.PostPRComments || rt.gitlabAppSecrets == nil {
		return "", "", "", false
	}
	conn, accessToken, err := rt.gitlabAccessToken(ctx)
	if err != nil {
		rt.logger.Info("api: preview gitlab notification skipped: no usable gitlab connection",
			slog.String("app_name", appName), slog.String("error", err.Error()))
		return "", "", "", false
	}
	projectPath, ok = gitlabProjectPathFromURL(gs.RepoURL, conn.InstanceURL)
	if !ok {
		rt.logger.Info("api: preview gitlab notification skipped: repo_url is not on the connected gitlab instance",
			slog.String("app_name", appName), slog.String("repo_url", gs.RepoURL))
		return "", "", "", false
	}
	return conn.InstanceURL, accessToken, projectPath, true
}

// gitlabProjectPathFromURL extracts a GitLab project's namespaced path
// ("group/project", or "group/subgroup/project") from repoURL, valid
// only when it's an https URL on instanceURL's own host, mirroring
// githubOwnerRepoFromURL's own validation shape.
func gitlabProjectPathFromURL(repoURL, instanceURL string) (path string, ok bool) {
	inst, err := url.Parse(instanceURL)
	if err != nil {
		return "", false
	}
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host != inst.Host {
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if path == "" {
		return "", false
	}
	return path, true
}

// notifyPreviewPendingGitLab sets a pending commit status on headSHA
// right before a preview deploy/redeploy actually starts building,
// mirroring notifyPreviewPendingGitHub's own reasoning.
func (rt *Router) notifyPreviewPendingGitLab(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	instanceURL, accessToken, projectPath, ok := rt.previewGitLabTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.gitlabAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, projectPath, headSHA,
		gitlabapp.CommitStatePending, "", "Deploying preview environment...", previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview pending gitlab commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewSuccessGitLab posts the live preview URL as a merge
// request note and sets a success commit status pointing at it,
// mirroring notifyPreviewSuccessGitHub's own reasoning.
func (rt *Router) notifyPreviewSuccessGitLab(ctx context.Context, appName string, gs store.GitSource, mrIID int, headSHA, previewURL string) {
	instanceURL, accessToken, projectPath, ok := rt.previewGitLabTarget(ctx, appName, gs)
	if !ok {
		return
	}

	description := "Preview deployed"
	targetURL := ""
	body := fmt.Sprintf("Preview environment deployed for commit `%s`.", headSHA)
	if previewURL != "" {
		targetURL = "https://" + previewURL
		description = "Preview deployed: " + previewURL
		body = fmt.Sprintf("Preview environment deployed for commit `%s`: %s", headSHA, targetURL)
	}

	if err := rt.gitlabAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, projectPath, headSHA,
		gitlabapp.CommitStateSuccess, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success gitlab commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
	if err := rt.gitlabAppClient.CreateMergeRequestNote(ctx, instanceURL, accessToken, projectPath, mrIID, body); err != nil {
		rt.logger.Warn("api: post preview success gitlab merge request note failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("mr_iid", mrIID))
	}
}

// notifyPreviewFailureGitLab sets a failure commit status, no merge
// request note, mirroring notifyPreviewFailureGitHub's own reasoning.
func (rt *Router) notifyPreviewFailureGitLab(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	instanceURL, accessToken, projectPath, ok := rt.previewGitLabTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.gitlabAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, projectPath, headSHA,
		gitlabapp.CommitStateFailed, "", truncateStatusDescription(reason, gitLabStatusDescriptionMax), previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview failure gitlab commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewTornDownGitLab posts a teardown notice as a merge request
// note, mirroring notifyPreviewTornDownGitHub's own reasoning.
func (rt *Router) notifyPreviewTornDownGitLab(ctx context.Context, preview store.PreviewEnvironment, gs store.GitSource) {
	instanceURL, accessToken, projectPath, ok := rt.previewGitLabTarget(ctx, preview.AppName, gs)
	if !ok {
		return
	}
	body := fmt.Sprintf("Preview environment `%s` torn down.", preview.PreviewAppID)
	if err := rt.gitlabAppClient.CreateMergeRequestNote(ctx, instanceURL, accessToken, projectPath, preview.PRNumber, body); err != nil {
		rt.logger.Warn("api: post preview teardown gitlab merge request note failed", slog.String("error", err.Error()), slog.String("app_name", preview.AppName), slog.Int("pr_number", preview.PRNumber))
	}
}
