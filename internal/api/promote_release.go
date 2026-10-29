package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Binding change reasons written to binding history.
const (
	bindingReasonPromote  = release.ReasonPromote
	bindingReasonRollback = release.ReasonRollback
	bindingReasonRevert   = release.ReasonRevert
)

// releaseBinder is the store surface that makes bindings authoritative.
type releaseBinder = release.Binder

// authoritativeBinder returns the store only when APP_RELEASE_BINDINGS=on, so
// off and shadow never reach a binding code path.
func (rt *Router) authoritativeBinder() releaseBinder {
	if !release.ModeFromEnv().Authoritative() {
		return nil
	}
	b, _ := rt.apps.(releaseBinder)
	return b
}

// promotionRelease returns the release source is bound to, or nil when
// bindings are not authoritative or the source has none yet.
func (rt *Router) promotionRelease(ctx context.Context, source store.DesiredService) *store.Release {
	b := rt.authoritativeBinder()
	if b == nil {
		return nil
	}
	return release.PromotionRelease(ctx, b, source.Name, rt.logger)
}

// retargetBinding is release.Retarget with the router's audit log.
func (rt *Router) retargetBinding(ctx context.Context, target store.DesiredService, releaseID, reason, by, approvalID string) (func(), error) {
	b := rt.authoritativeBinder()
	if b == nil {
		return func() {}, nil
	}
	return release.Retarget(ctx, b, rt.auditSystemEvent, rt.logger, target, releaseID, reason, by, approvalID)
}

// rollbackReleaseID finds the release a past attempt ran.
func (rt *Router) rollbackReleaseID(ctx context.Context, target store.DeployAttempt) string {
	b := rt.authoritativeBinder()
	if b == nil {
		return ""
	}
	return release.RollbackReleaseID(ctx, b, target)
}

// applyReleaseApproval resolves the image an approval applies from its stored
// release, so a tag that moved since the request cannot change the content.
func (rt *Router) applyReleaseApproval(ctx context.Context, a store.DeployApproval) (string, *deployApprovalDecisionError) {
	b, _ := rt.apps.(releaseBinder)
	if b == nil {
		return "", &deployApprovalDecisionError{http.StatusConflict, "release " + a.ReleaseID + " cannot be read; reject the approval and request again"}
	}
	rel, err := b.GetRelease(ctx, a.ReleaseID)
	if err != nil || rel == nil {
		return "", &deployApprovalDecisionError{http.StatusConflict, "release " + a.ReleaseID + " no longer exists; reject the approval and request again"}
	}
	return rel.ImageRef, nil
}

// actorLabel names the caller for binding history; empty when unknown.
func (rt *Router) actorLabel(r *http.Request) string {
	_, _, name, _ := rt.currentActor(r)
	return name
}

// requestedRelease loads an explicit release_id and trusts it only after the
// app-ownership check; a foreign or unknown id is a 404 so ids do not leak.
func (rt *Router) requestedRelease(w http.ResponseWriter, r *http.Request, app, id string) (*store.Release, bool) {
	b := rt.authoritativeBinder()
	if b == nil {
		writeError(w, http.StatusBadRequest, "release_id needs release bindings enabled (APP_RELEASE_BINDINGS=on)")
		return nil, false
	}
	rel, err := b.GetRelease(r.Context(), id)
	if err != nil || rel == nil || !release.OwnsRelease(r.Context(), b, app, rel) {
		writeError(w, http.StatusNotFound, "release not found for this app")
		return nil, false
	}
	return rel, true
}
