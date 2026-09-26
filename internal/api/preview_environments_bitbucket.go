package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewBitbucketTarget resolves an access token and gs's own
// "workspace/repo_slug" full name. ok is false, treated as a silent
// no-op by every caller, whenever gs.PostPRComments is off, no
// Bitbucket OAuth consumer is authorized, or gs.RepoURL isn't hosted on
// bitbucket.org (Cloud only, mirrors previewGitHubTarget).
func (rt *Router) previewBitbucketTarget(ctx context.Context, appName string, gs store.GitSource) (accessToken, fullName string, ok bool) {
	if !gs.PostPRComments || rt.bitbucketAppSecrets == nil {
		return "", "", false
	}
	accessToken, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		rt.logger.Info("api: preview bitbucket notification skipped: no usable bitbucket connection",
			slog.String("app_name", appName), slog.String("error", err.Error()))
		return "", "", false
	}
	fullName, ok = bitbucketFullNameFromURL(gs.RepoURL)
	if !ok {
		rt.logger.Info("api: preview bitbucket notification skipped: repo_url is not on bitbucket.org",
			slog.String("app_name", appName), slog.String("repo_url", gs.RepoURL))
		return "", "", false
	}
	return accessToken, fullName, true
}

// bitbucketFullNameFromURL extracts a "workspace/repo_slug" full name
// from repoURL, valid only when it's an https URL on bitbucket.org,
// mirroring githubOwnerRepoFromURL's own validation shape.
func bitbucketFullNameFromURL(repoURL string) (fullName string, ok bool) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "bitbucket.org") {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), true
}

// notifyPreviewPendingBitbucket sets an in-progress build status on
// headSHA right before a preview deploy/redeploy actually starts
// building, mirroring notifyPreviewPendingGitHub's own reasoning.
func (rt *Router) notifyPreviewPendingBitbucket(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	accessToken, fullName, ok := rt.previewBitbucketTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.bitbucketAppClient.CreateCommitBuildStatus(ctx, accessToken, fullName, headSHA,
		bitbucketapp.BuildStatusInProgress, "", "Deploying preview environment...", previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview pending bitbucket build status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewSuccessBitbucket sets a success commit status pointing at the live
// preview URL. The PR comment is upserted separately (upsertPreviewComment).
func (rt *Router) notifyPreviewSuccessBitbucket(ctx context.Context, appName string, gs store.GitSource, headSHA, previewURL string) {
	accessToken, fullName, ok := rt.previewBitbucketTarget(ctx, appName, gs)
	if !ok {
		return
	}

	description := "Preview deployed"
	targetURL := ""
	if previewURL != "" {
		targetURL = "https://" + previewURL
		description = "Preview deployed: " + previewURL
	}

	if err := rt.bitbucketAppClient.CreateCommitBuildStatus(ctx, accessToken, fullName, headSHA,
		bitbucketapp.BuildStatusSuccessful, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success bitbucket build status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewFailureBitbucket sets a failed build status. The failure reason
// also lands in the single PR comment (upsertPreviewComment).
// reason is sent untruncated: this provider documents no hard limit for the
// status description.
func (rt *Router) notifyPreviewFailureBitbucket(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	accessToken, fullName, ok := rt.previewBitbucketTarget(ctx, appName, gs)
	if !ok {
		return
	}
	if err := rt.bitbucketAppClient.CreateCommitBuildStatus(ctx, accessToken, fullName, headSHA,
		bitbucketapp.BuildStatusFailed, "", reason, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview failure bitbucket build status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}
