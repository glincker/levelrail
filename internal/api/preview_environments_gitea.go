package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewGiteaTarget resolves an access token and gs's own owner/repo.
// ok is false, treated as a silent no-op by every caller, whenever
// gs.PostPRComments is off, no Gitea OAuth2 application is authorized,
// or gs.RepoURL isn't hosted on the connected instance (mirrors
// previewGitHubTarget).
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

// notifyPreviewSuccessGitea sets a success commit status pointing at the live
// preview URL. The PR comment is upserted separately (upsertPreviewComment).
func (rt *Router) notifyPreviewSuccessGitea(ctx context.Context, appName string, gs store.GitSource, headSHA, previewURL string) {
	instanceURL, accessToken, fullName, ok := rt.previewGiteaTarget(ctx, appName, gs)
	if !ok {
		return
	}

	description := "Preview deployed"
	targetURL := ""
	if previewURL != "" {
		targetURL = "https://" + previewURL
		description = "Preview deployed: " + previewURL
	}

	if err := rt.giteaAppClient.CreateCommitStatus(ctx, instanceURL, accessToken, fullName, headSHA,
		giteaapp.CommitStatusSuccess, targetURL, description, previewStatusContext); err != nil {
		rt.logger.Warn("api: post preview success gitea commit status failed", slog.String("error", err.Error()), slog.String("app_name", appName))
	}
}

// notifyPreviewFailureGitea sets a failure commit status. The failure reason
// also lands in the single PR comment (upsertPreviewComment).
// reason is sent untruncated: this provider documents no hard limit for the
// status description.
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
