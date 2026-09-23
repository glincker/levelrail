package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// previewStatusContext is the provider-agnostic "context"/"name"/"key"
// string every provider's preview commit status is grouped under,
// distinct from any CI status a repo's own pipeline already posts for
// the same commit.
const previewStatusContext = "levelrail/preview"

// truncateStatusDescription caps s at limit characters, appending "..."
// when it's cut, matching a provider's own documented hard limit for a
// commit/build status description field (GitHub: 140, GitLab: 255).
func truncateStatusDescription(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit-3] + "..."
}

// notifyPreviewPending fans out a pending commit-status notification to
// every provider gs might be hosted on. Each provider's own
// notifyPreviewPending<Provider> is a no-op unless gs.RepoURL actually
// matches that provider's connected instance, so exactly one of these
// four calls ever does real work for a given git source.
func (rt *Router) notifyPreviewPending(ctx context.Context, appName string, gs store.GitSource, headSHA string) {
	rt.notifyPreviewPendingGitHub(ctx, appName, gs, headSHA)
	rt.notifyPreviewPendingGitLab(ctx, appName, gs, headSHA)
	rt.notifyPreviewPendingBitbucket(ctx, appName, gs, headSHA)
	rt.notifyPreviewPendingGitea(ctx, appName, gs, headSHA)
}

// notifyPreviewSuccess fans out a success commit-status and PR/MR
// comment notification, the same "exactly one provider matches" shape
// notifyPreviewPending's own doc comment describes.
func (rt *Router) notifyPreviewSuccess(ctx context.Context, appName string, gs store.GitSource, prNumber int, headSHA, previewURL string) {
	rt.notifyPreviewSuccessGitHub(ctx, appName, gs, prNumber, headSHA, previewURL)
	rt.notifyPreviewSuccessGitLab(ctx, appName, gs, prNumber, headSHA, previewURL)
	rt.notifyPreviewSuccessBitbucket(ctx, appName, gs, prNumber, headSHA, previewURL)
	rt.notifyPreviewSuccessGitea(ctx, appName, gs, prNumber, headSHA, previewURL)
}

// notifyPreviewFailure fans out a failure commit-status notification,
// the same "exactly one provider matches" shape notifyPreviewPending's
// own doc comment describes.
func (rt *Router) notifyPreviewFailure(ctx context.Context, appName string, gs store.GitSource, headSHA, reason string) {
	rt.notifyPreviewFailureGitHub(ctx, appName, gs, headSHA, reason)
	rt.notifyPreviewFailureGitLab(ctx, appName, gs, headSHA, reason)
	rt.notifyPreviewFailureBitbucket(ctx, appName, gs, headSHA, reason)
	rt.notifyPreviewFailureGitea(ctx, appName, gs, headSHA, reason)
}

// notifyPreviewTornDown fans out a teardown notice, the same "exactly
// one provider matches" shape notifyPreviewPending's own doc comment
// describes. Loads gs once here (rather than each provider's own
// function loading it independently) since teardown is called from
// three sites (the pull-request-closed webhook, the manual teardown
// route, and the TTL sweep), only one of which already has gs in scope.
func (rt *Router) notifyPreviewTornDown(ctx context.Context, preview store.PreviewEnvironment) {
	gs, err := rt.gitSources.GetGitSource(ctx, preview.AppName)
	if err != nil {
		return
	}
	rt.notifyPreviewTornDownGitHub(ctx, preview, *gs)
	rt.notifyPreviewTornDownGitLab(ctx, preview, *gs)
	rt.notifyPreviewTornDownBitbucket(ctx, preview, *gs)
	rt.notifyPreviewTornDownGitea(ctx, preview, *gs)
}
