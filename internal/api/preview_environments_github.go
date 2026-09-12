package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewGitHubStatusContext is the "context" string GitHub shows next
// to every commit status this file posts, grouping them under one named
// check distinct from any CI status a repo's own Actions workflow
// already posts for the same commit.
const previewGitHubStatusContext = "levelrail/preview"

// githubStatusDescriptionMax is GitHub's own documented limit for a
// commit status's description field.
const githubStatusDescriptionMax = 140

// previewGitHubTarget resolves what CreateIssueComment/CreateCommitStatus
// need to notify GitHub about gs's preview: an installation token and
// gs's own owner/repo. ok is false, and every caller here treats that as
// a silent no-op, whenever gs.PostPRComments is off, no GitHub App
// installation is usable, or gs.RepoURL isn't actually hosted on that
// installation's own instance (a GitLab/Bitbucket source, or a repo
// added before an App was ever connected): posting a comment or status
// is cosmetic to the preview it annotates, never a precondition for the
// preview itself, the same "logged, not fatal" shape saveWebhookDelivery
// (git_webhook.go) already establishes for a different non-critical side
// effect of the same webhook path.
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
// GitLab/Bitbucket repo_url, or a malformed one, is never mistaken for a
// GitHub one.
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

// truncateForGitHubStatus caps reason at githubStatusDescriptionMax,
// GitHub's own hard limit for a commit status description: a preview
// deploy failure's own error string can be much longer, and GitHub
// rejects the whole request outright when it is.
func truncateForGitHubStatus(reason string) string {
	if len(reason) <= githubStatusDescriptionMax {
		return reason
	}
	return reason[:githubStatusDescriptionMax-3] + "..."
}

// notifyPreviewPending sets a pending commit status on headSHA right
// before a preview deploy/redeploy actually starts building. Best-effort
// and logged only: a failed status post must never fail the preview
// deploy it's merely annotating.
func (rt *Router) notifyPreviewPending(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusPending, "", "Deploying preview environment...", previewGitHubStatusContext); err != nil {
		rt.logger.Warn("api: post preview pending commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewSuccess posts the live preview URL as a PR comment and
// sets a success commit status pointing at it: the two GitHub-visible
// signals a successful preview deploy/redeploy produces. previewURL is
// "" when no control-plane primary domain is configured (previewDomain's
// own doc comment): the comment and status still post, just without a
// link to follow.
func (rt *Router) notifyPreviewSuccess(ctx context.Context, appName string, gs store.GitSource, prNumber int, headSHA, previewURL string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
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

	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusSuccess, targetURL, description, previewGitHubStatusContext); err != nil {
		rt.logger.Warn("api: post preview success commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
	if err := rt.githubAppClient.CreateIssueComment(ctx, instanceURL, token, owner, repo, prNumber, body); err != nil {
		rt.logger.Warn("api: post preview success pr comment failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", prNumber))
	}
}

// notifyPreviewFailure sets a failure commit status. No PR comment,
// unlike notifyPreviewSuccess/notifyPreviewTornDown: GET .../previews
// already surfaces a failed deploy's own reason through StatusReason,
// and a failing build commonly gets redeployed on the very next push,
// which would otherwise leave a stale failure comment sitting on the PR.
func (rt *Router) notifyPreviewFailure(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.githubAppClient.CreateCommitStatus(ctx, instanceURL, token, owner, repo, headSHA,
		githubapp.CommitStatusFailure, "", truncateForGitHubStatus(reason), previewGitHubStatusContext); err != nil {
		rt.logger.Warn("api: post preview failure commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewTornDown posts a teardown notice on the PR: the comment
// counterpart to a torn-down preview's own commit status, which is left
// alone (the last pending/success/failure status a closed PR had stays
// exactly as GitHub already shows it). Loads gs itself from
// preview.AppName rather than taking one as a parameter: this is called
// from three sites (the pull-request-closed webhook, the manual teardown
// route, and the TTL sweep), and only one of them already has gs in
// scope, so a single lookup here is simpler than threading it through
// all three.
func (rt *Router) notifyPreviewTornDown(ctx context.Context, preview store.PreviewEnvironment) {
	gs, err := rt.gitSources.GetGitSource(ctx, preview.AppName)
	if err != nil {
		return
	}
	instanceURL, token, owner, repo, ok := rt.previewGitHubTarget(ctx, preview.AppName, *gs)
	if !ok {
		return
	}
	body := fmt.Sprintf("Preview environment `%s` torn down.", preview.PreviewAppID)
	if err := rt.githubAppClient.CreateIssueComment(ctx, instanceURL, token, owner, repo, preview.PRNumber, body); err != nil {
		rt.logger.Warn("api: post preview teardown pr comment failed", slog.String("error", err.Error()), slog.String("app_name", preview.AppName), slog.Int("pr_number", preview.PRNumber))
	}
}
