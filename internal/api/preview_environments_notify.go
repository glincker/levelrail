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

// notifyPreviewSuccess fans out a success commit status, the same
// "exactly one provider matches" shape notifyPreviewPending's own doc
// comment describes.
func (rt *Router) notifyPreviewSuccess(ctx context.Context, appName string, gs store.GitSource, headSHA, previewURL string) {
	rt.notifyPreviewSuccessGitHub(ctx, appName, gs, headSHA, previewURL)
	rt.notifyPreviewSuccessGitLab(ctx, appName, gs, headSHA, previewURL)
	rt.notifyPreviewSuccessBitbucket(ctx, appName, gs, headSHA, previewURL)
	rt.notifyPreviewSuccessGitea(ctx, appName, gs, headSHA, previewURL)
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

// notifyPreviewRemoved edits the pull request's status comment to say the
// preview is gone and why.
func (rt *Router) notifyPreviewRemoved(ctx context.Context, preview store.PreviewEnvironment, reason string) {
	gs, err := rt.gitSources.GetGitSource(ctx, preview.AppName)
	if err != nil {
		return
	}
	rt.upsertPreviewComment(ctx, *gs, &preview, previewCommentRemoved, reason)
}
