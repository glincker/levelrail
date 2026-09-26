package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gitLabStatusDescriptionMax is GitLab's own documented limit for a
// commit status's description field.
const gitLabStatusDescriptionMax = 255

// previewGitLabTarget resolves an access token and gs's own
// URL-encoded project path. ok is false, treated as a silent no-op by
// every caller, whenever gs.PostPRComments is off, no GitLab OAuth
// application is authorized, or gs.RepoURL isn't hosted on the
// connected instance (mirrors previewGitHubTarget).
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

// notifyPreviewSuccessGitLab sets a success commit status pointing at the live
// preview URL. The PR comment is upserted separately (upsertPreviewComment).
func (rt *Router) notifyPreviewSuccessGitLab(ctx context.Context, appName string, gs store.GitSource, headSHA, previewURL string) {
	instanceURL, accessToken, projectPath, ok := rt.previewGitLabTarget(ctx, appName, gs)
	if !ok {
		return
	}

	description := "Preview deployed"
	targetURL := ""
	if previewURL != "" {
		targetURL = "https://" + previewURL
		description = "Preview deployed: " + previewURL
	}

	if err := rt.gitlabAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, projectPath, headSHA,
		gitlabapp.CommitStateSuccess, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success gitlab commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewFailureGitLab sets a failure commit status. The failure reason
// also lands in the single PR comment (upsertPreviewComment).
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
