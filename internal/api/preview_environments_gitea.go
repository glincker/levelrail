package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewGiteaTarget resolves what CreateIssueComment/CreateCommitStatus
// need to notify Gitea about gs's preview: an OAuth access token and
// gs's own owner/repo. ok is false, and every caller here treats that
// as a silent no-op, whenever gs.PostPRComments is off, no Gitea OAuth2
// application is connected and authorized, or gs.RepoURL isn't actually
// hosted on the connected instance: the same "logged, not fatal" shape
// previewGitHubTarget already establishes.
func (rt *Router) previewGiteaTarget(ctx context.Context, appName string, gs store.GitSource) (instanceURL, accessToken, fullName string, ok bool) {
	if !gs.PostPRComments || rt.giteaAppSecrets == nil {
		return "", "", "", false
	}
	conn, accessToken, err := rt.giteaAccessToken(ctx)
	if err != nil {
		rt.logger.Info("api: preview gitea notification skipped: no usable gitea connection",
			slog.String("app_name", appName), slog.String("error", err.Error()))
		return "", "", "", false
	}
	fullName, ok = giteaFullNameFromURL(gs.RepoURL, conn.InstanceURL)
	if !ok {
		rt.logger.Info("api: preview gitea notification skipped: repo_url is not on the connected gitea instance",
			slog.String("app_name", appName), slog.String("repo_url", gs.RepoURL))
		return "", "", "", false
	}
	return conn.InstanceURL, accessToken, fullName, true
}

// giteaFullNameFromURL extracts an "owner/repo" full name from repoURL,
// valid only when it's an https URL on instanceURL's own host, mirroring
// githubOwnerRepoFromURL's own validation shape.
func giteaFullNameFromURL(repoURL, instanceURL string) (fullName string, ok bool) {
	inst, err := url.Parse(instanceURL)
	if err != nil {
		return "", false
	}
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host != inst.Host {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), true
}

// notifyPreviewPendingGitea sets a pending commit status on headSHA
// right before a preview deploy/redeploy actually starts building,
// mirroring notifyPreviewPendingGitHub's own reasoning.
func (rt *Router) notifyPreviewPendingGitea(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	instanceURL, accessToken, fullName, ok := rt.previewGiteaTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.giteaAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, fullName, headSHA,
		giteaapp.CommitStatusPending, "", "Deploying preview environment...", previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview pending gitea commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewSuccessGitea posts the live preview URL as an issue
// comment and sets a success commit status pointing at it, mirroring
// notifyPreviewSuccessGitHub's own reasoning.
func (rt *Router) notifyPreviewSuccessGitea(ctx context.Context, appName string, gs store.GitSource, prNumber int, headSHA, previewURL string) {
	instanceURL, accessToken, fullName, ok := rt.previewGiteaTarget(ctx, appName, gs)
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

	if err := rt.giteaAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, fullName, headSHA,
		giteaapp.CommitStatusSuccess, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success gitea commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
	if err := rt.giteaAppClient.CreateIssueComment(ctx, instanceURL, accessToken, fullName, prNumber, body); err != nil {
		rt.logger.Warn("api: post preview success gitea pr comment failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", prNumber))
	}
}

// notifyPreviewFailureGitea sets a failure commit status, no issue
// comment, mirroring notifyPreviewFailureGitHub's own reasoning. reason
// is sent untruncated: Gitea's commit status "description" field has no
// documented hard limit the way GitHub's (140) does.
func (rt *Router) notifyPreviewFailureGitea(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	instanceURL, accessToken, fullName, ok := rt.previewGiteaTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.giteaAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, fullName, headSHA,
		giteaapp.CommitStatusFailure, "", reason, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview failure gitea commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewTornDownGitea posts a teardown notice as an issue
// comment, mirroring notifyPreviewTornDownGitHub's own reasoning.
func (rt *Router) notifyPreviewTornDownGitea(ctx context.Context, preview store.PreviewEnvironment, gs store.GitSource) {
	instanceURL, accessToken, fullName, ok := rt.previewGiteaTarget(ctx, preview.AppName, gs)
	if !ok {
		return
	}
	body := fmt.Sprintf("Preview environment `%s` torn down.", preview.PreviewAppID)
	if err := rt.giteaAppClient.CreateIssueComment(ctx, instanceURL, accessToken, fullName, preview.PRNumber, body); err != nil {
		rt.logger.Warn("api: post preview teardown gitea pr comment failed", slog.String("error", err.Error()), slog.String("app_name", preview.AppName), slog.Int("pr_number", preview.PRNumber))
	}
}
