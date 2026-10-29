package release

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Binding change reasons written to binding history.
const (
	ReasonPromote  = "promote"
	ReasonRollback = "rollback"
	ReasonRevert   = "revert"
	ReasonDeploy   = "deploy"
)

// Binder is the store surface that makes bindings authoritative. It lives here
// so the API and the pipeline runner retarget through one code path.
type Binder interface {
	GetRelease(ctx context.Context, id string) (*store.Release, error)
	GetReleaseByAttempt(ctx context.Context, attemptID string) (*store.Release, error)
	GetEnvironmentBinding(ctx context.Context, app string) (*store.EnvironmentBinding, error)
	ListBindingHistory(ctx context.Context, app string) ([]store.BindingHistoryEntry, error)
	RetargetBinding(ctx context.Context, app, environmentID, releaseID, reason, by, approvalID string) (bool, error)
	RetargetBindingIf(ctx context.Context, app, environmentID, releaseID, expected, reason, by, approvalID string) (bool, error)
	UnbindRelease(ctx context.Context, app, expected, by string) (bool, error)
}

// Auditor records one platform-initiated event.
type Auditor func(ctx context.Context, actor, action, detail string)

// AuditorFor writes system audit rows straight to the store, for callers that
// have no API router.
func AuditorFor(db *store.DB, logger *slog.Logger) Auditor {
	return func(ctx context.Context, actor, action, detail string) {
		id, err := store.NewAuditEntryID()
		if err != nil {
			return
		}
		err = db.SaveAuditEntry(ctx, store.AuditEntry{
			ID: id, ActorType: "system", ActorName: actor, Ability: "root", Method: http.MethodPost,
			Path: action + ": " + detail, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()),
			ClientKind: "system", Action: action,
		})
		if err != nil && logger != nil {
			logger.Warn("release: system audit failed", slog.String("action", action), slog.String("error", err.Error()))
		}
	}
}

// PromotionRelease returns the release source is bound to, or nil when it has
// none (the tag path then applies, as the ADR's no-digest fallback).
func PromotionRelease(ctx context.Context, b Binder, source string, logger *slog.Logger) *store.Release {
	if b == nil {
		return nil
	}
	bind, err := b.GetEnvironmentBinding(ctx, source)
	if err != nil || bind == nil || bind.ReleaseID == "" {
		return nil
	}
	rel, err := b.GetRelease(ctx, bind.ReleaseID)
	if err != nil || rel == nil {
		if logger != nil {
			logger.Warn("release: promote: load source release failed", slog.String("name", source))
		}
		return nil
	}
	return rel
}

// Retarget points target's binding at releaseID, audits the move and returns
// an undo for the caller to run if the deploy then fails, so the binding never
// names a release that did not run. The undo only applies while the binding
// still names releaseID, so a newer retarget is never reverted.
func Retarget(ctx context.Context, b Binder, audit Auditor, logger *slog.Logger, target store.DesiredService, releaseID, reason, by, approvalID string) (func(), error) {
	noop := func() {}
	if b == nil || releaseID == "" {
		return noop, nil
	}
	cur, err := b.GetEnvironmentBinding(ctx, target.Name)
	if err != nil {
		return noop, fmt.Errorf("read binding for %q: %w", target.Name, err)
	}
	prev := ""
	if cur != nil {
		prev = cur.ReleaseID
	}
	moved, err := b.RetargetBinding(ctx, target.Name, target.EnvironmentID, releaseID, reason, by, approvalID)
	if err != nil {
		return noop, fmt.Errorf("retarget binding for %q: %w", target.Name, err)
	}
	if !moved {
		return noop, nil
	}
	emit := func(ctx context.Context, action, detail string) {
		if audit != nil {
			audit(ctx, "release-binding", action, detail)
		}
	}
	emit(ctx, "release_binding.retargeted",
		fmt.Sprintf("app=%s from=%s to=%s reason=%s approval=%s by=%s", target.Name, prev, releaseID, reason, approvalID, by))
	return func() {
		// A cancelled request must still be able to undo its own move.
		ctx := context.WithoutCancel(ctx)
		if prev == "" {
			done, err := b.UnbindRelease(ctx, target.Name, releaseID, by)
			finishUndo(logger, target.Name, done, err)
			if done {
				emit(ctx, "release_binding.unbound",
					fmt.Sprintf("app=%s release=%s approval=%s by=%s", target.Name, releaseID, approvalID, by))
			}
			return
		}
		done, err := b.RetargetBindingIf(ctx, target.Name, target.EnvironmentID, prev, releaseID, ReasonRevert, by, approvalID)
		finishUndo(logger, target.Name, done, err)
		if done {
			emit(ctx, "release_binding.retargeted",
				fmt.Sprintf("app=%s from=%s to=%s reason=%s approval=%s by=%s", target.Name, releaseID, prev, ReasonRevert, approvalID, by))
		}
	}, nil
}

func finishUndo(logger *slog.Logger, app string, done bool, err error) {
	if logger == nil {
		return
	}
	switch {
	case err != nil:
		logger.Warn("release: undo binding failed", slog.String("name", app), slog.String("error", err.Error()))
	case !done:
		logger.Info("release: undo binding skipped, a newer retarget won", slog.String("name", app))
	}
}

// RollbackReleaseID finds the release a past attempt ran: the one minted from
// it, else a bound release of the app with the same digest pinned ref.
func RollbackReleaseID(ctx context.Context, b Binder, target store.DeployAttempt) string {
	if b == nil {
		return ""
	}
	if rel, err := b.GetReleaseByAttempt(ctx, target.ID); err == nil && rel != nil {
		return rel.ID
	}
	if target.ImageDigest == "" {
		return ""
	}
	want := ImageRef(target.Image, target.ImageDigest)
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

// OwnsRelease reports whether rel may be applied to app: it was minted from
// app, or app's binding history has named it.
func OwnsRelease(ctx context.Context, b Binder, app string, rel *store.Release) bool {
	if rel == nil || b == nil {
		return false
	}
	if rel.SourceApp == app {
		return true
	}
	hist, err := b.ListBindingHistory(ctx, app)
	if err != nil {
		return false
	}
	for _, h := range hist {
		if h.ReleaseID == rel.ID {
			return true
		}
	}
	return false
}
