package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// githubStatusDescriptionMax is GitHub's own documented limit for a
// commit status's description field.
const githubStatusDescriptionMax = 140

// previewGitHubTarget resolves what CreateIssueComment/CreateCommitStatus
// need to notify GitHub about gs's preview: an installation token and
// gs's own owner/repo. ok is false, and every caller here treats that as
// a silent no-op, whenever gs.PostPRComments is off, no GitHub App
// installation is usable, or gs.RepoURL isn't actually hosted on that
// installation's own instance (a GitLab/Bitbucket/Gitea source, or a
// repo added before an App was ever connected): posting a comment or
// status is cosmetic to the preview it annotates, never a precondition
// for the preview itself, the same "logged, not fatal" shape
// saveWebhookDelivery (git_webhook.go) already establishes for a
// different non-critical side effect of the same webhook path.
func (rt *Router) previewGitHubTarget(ctx context.Context, appName string, gs store.GitSource) (instanceURL, token, owner, repo string, ok bool) {
	if !gs.PostPRComments || rt.githubAppSecrets == nil {
		return "", "", "", "", false
	}
	instanceURL, token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.logger.Info("api: preview github notification skipped: no usable github app installation",
			slog.String("app_name", appName), slog.String("error", err.Error()))
		return "", "", "", "", false
	}
	owner, repo, ok = githubOwnerRepoFromURL(gs.RepoURL, instanceURL)
	if !ok {
		rt.logger.Info("api: preview github notification skipped: repo_url is not on the connected github instance",
			slog.String("app_name", appName), slog.String("repo_url", gs.RepoURL))
		return "", "", "", "", false
	}
	return instanceURL, token, owner, repo, true
}

// githubOwnerRepoFromURL extracts owner/repo from repoURL, valid only
// when it's an https URL on instanceURL's own host and shaped exactly
// like ".../<owner>/<repo>[.git]": the same shape isGitHubHTTPSRepoURL
// (builds.go) already validates for the git-clone path, reused here so a
// GitLab/Bitbucket/Gitea repo_url, or a malformed one, is never mistaken
// for a GitHub one.
func githubOwnerRepoFromURL(repoURL, instanceURL string) (owner, repo string, ok bool) {
	inst, err := url.Parse(instanceURL)
	if err != nil {
		return "", "", false
	}
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host != inst.Host {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}

// notifyPreviewPendingGitHub sets a pending commit status on headSHA
// right before a preview deploy/redeploy actually starts building.
// Best-effort and logged only: a failed status post must never fail the
// preview deploy it's merely annotating.
func (rt *Router) notifyPreviewPendingGitHub(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusPending, "", "Deploying preview environment...", previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview pending commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewSuccessGitHub sets a success commit status pointing at the live
// preview URL. The PR comment is upserted separately (upsertPreviewComment).
func (rt *Router) notifyPreviewSuccessGitHub(ctx context.Context, appName string, gs store.GitSource, headSHA, previewURL string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
	if !ok {
		return
	}

	description := "Preview deployed"
	targetURL := ""
	if previewURL != "" {
		targetURL = "https://" + previewURL
		description = "Preview deployed: " + previewURL
	}

	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusSuccess, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewFailureGitHub sets a failure commit status. The failure reason
// also lands in the single PR comment (upsertPreviewComment).
func (rt *Router) notifyPreviewFailureGitHub(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusFailure, "", truncateStatusDescription(reason, githubStatusDescriptionMax), previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview failure commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}
