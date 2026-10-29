package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Binding change reasons written to binding history.
const (
	bindingReasonPromote  = "promote"
	bindingReasonRollback = "rollback"
	bindingReasonRevert   = "revert"
)

// releaseBinder is the store surface that makes bindings authoritative.
type releaseBinder interface {
	GetRelease(ctx context.Context, id string) (*store.Release, error)
	GetReleaseByAttempt(ctx context.Context, attemptID string) (*store.Release, error)
	GetEnvironmentBinding(ctx context.Context, app string) (*store.EnvironmentBinding, error)
	ListBindingHistory(ctx context.Context, app string) ([]store.BindingHistoryEntry, error)
	RetargetBinding(ctx context.Context, app, environmentID, releaseID, reason, by, approvalID string) (bool, error)
}

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
// bindings are not authoritative or the source has none yet (the tag path
// then applies, as the ADR's no-digest fallback).
func (rt *Router) promotionRelease(ctx context.Context, source store.DesiredService) *store.Release {
	b := rt.authoritativeBinder()
	if b == nil {
		return nil
	}
	bind, err := b.GetEnvironmentBinding(ctx, source.Name)
	if err != nil || bind == nil || bind.ReleaseID == "" {
		return nil
	}
	rel, err := b.GetRelease(ctx, bind.ReleaseID)
	if err != nil || rel == nil {
		rt.logger.Warn("api: promote: load source release failed", slog.String("name", source.Name))
		return nil
	}
	return rel
}

// retargetBinding points target's binding at releaseID, audits the move and
// returns an undo for the caller to run if the deploy then fails, so the
// binding never names a release that did not run.
func (rt *Router) retargetBinding(ctx context.Context, target store.DesiredService, releaseID, reason, by, approvalID string) (func(), error) {
	noop := func() {}
	b := rt.authoritativeBinder()
	if b == nil || releaseID == "" {
		return noop, nil
	}
	prev := ""
	if cur, err := b.GetEnvironmentBinding(ctx, target.Name); err == nil && cur != nil {
		prev = cur.ReleaseID
	}
	if _, err := b.RetargetBinding(ctx, target.Name, target.EnvironmentID, releaseID, reason, by, approvalID); err != nil {
		return noop, fmt.Errorf("api: retarget binding for %q: %w", target.Name, err)
	}
	rt.auditSystemEvent(ctx, "release-binding", "release_binding.retargeted",
		fmt.Sprintf("app=%s from=%s to=%s reason=%s approval=%s", target.Name, prev, releaseID, reason, approvalID))
	return func() {
		if prev == "" {
			if u, ok := rt.apps.(interface {
				UnbindRelease(ctx context.Context, app string) error
			}); ok {
				if err := u.UnbindRelease(ctx, target.Name); err != nil {
					rt.logger.Warn("api: undo binding failed", slog.String("name", target.Name), slog.String("error", err.Error()))
				}
			}
			return
		}
		if _, err := b.RetargetBinding(ctx, target.Name, target.EnvironmentID, prev, bindingReasonRevert, by, approvalID); err != nil {
			rt.logger.Warn("api: undo binding failed", slog.String("name", target.Name), slog.String("error", err.Error()))
			return
		}
		rt.auditSystemEvent(ctx, "release-binding", "release_binding.retargeted",
			fmt.Sprintf("app=%s from=%s to=%s reason=%s approval=%s", target.Name, releaseID, prev, bindingReasonRevert, approvalID))
	}, nil
}

// rollbackReleaseID finds the release a past attempt ran: the one minted from
// it, else a bound release of the app with the same digest pinned ref.
func (rt *Router) rollbackReleaseID(ctx context.Context, target store.DeployAttempt) string {
	b := rt.authoritativeBinder()
	if b == nil {
		return ""
	}
	if rel, err := b.GetReleaseByAttempt(ctx, target.ID); err == nil && rel != nil {
		return rel.ID
	}
	if target.ImageDigest == "" {
		return ""
	}
	want := release.ImageRef(target.Image, target.ImageDigest)
	hist, err := b.ListBindingHistory(ctx, target.ServiceName)
	if err != nil {
		return ""
	}
	for i := len(hist) - 1; i >= 0; i-- {
		if rel, err := b.GetRelease(ctx, hist[i].ReleaseID); err == nil && rel != nil && rel.ImageRef == want {
			return rel.ID
		}
	}
	return ""
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
