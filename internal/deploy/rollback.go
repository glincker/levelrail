package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageDeployStore is the narrow store surface TriggerImageDeploy needs:
// point a service's desired image at a new tag and record the
// deploy_attempts row for it. *store.DB satisfies this structurally.
type ImageDeployStore interface {
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error
}

// ReconcileNudger wakes the reconcile loop after a desired-state write,
// the same nudge every other deploy-trigger path already performs after
// its own SaveDesiredService call. *reconcile.Engine satisfies this
// structurally, the same "not imported directly" boundary this
// package's other narrow interfaces already draw.
type ReconcileNudger interface {
	Nudge()
}

// TriggerImageDeploy points existing's desired image at image, records a
// deploy_attempts row for it (source), and wakes the reconciler: the
// exact sequence internal/api/deploys.go's handleTriggerDeploy performs
// for every manual redeploy or manual rollback (POST
// /api/v1/apps/{name}/deploys). Factored out here so an automatic
// trigger, the crashloop auto-rollback in internal/alerting, drives
// production state through the identical path instead of a second,
// divergent one. nudger may be nil, in which case the caller relies on
// the next resync tick instead, the same "absence degrades, never
// errors" shape internal/api.Router's own reconcileNudger field follows.
func TriggerImageDeploy(ctx context.Context, st ImageDeployStore, nudger ReconcileNudger, existing store.DesiredService, image, source string, logger *slog.Logger) (store.DesiredService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	updated := existing
	updated.Image = image
	updated.EnvDirty = false
	if err := st.SaveDesiredService(ctx, updated); err != nil {
		return store.DesiredService{}, fmt.Errorf("deploy: trigger image deploy for %q: %w", existing.Name, err)
	}

	recordImageDeployAttempt(ctx, st, updated, image, source, logger)

	if nudger != nil {
		nudger.Nudge()
	}
	return updated, nil
}

// recordImageDeployAttempt saves and immediately finishes (as succeeded)
// a deploy_attempts row for a trigger with no build step to wait on,
// mirroring internal/api/deploys.go's recordInstantDeployAttempt. A
// failure here is logged, not returned: the desired-state write already
// succeeded, so a history-tracking hiccup must never turn an otherwise-
// successful deploy trigger into a caller-visible failure.
func recordImageDeployAttempt(ctx context.Context, st ImageDeployStore, svc store.DesiredService, image, source string, logger *slog.Logger) {
	id, err := store.NewDeployAttemptID()
	if err != nil {
		logger.Error("deploy: record deploy attempt: mint id failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return
	}
	now := time.Now()
	if err := st.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: svc.Name, Image: image,
		Source: source,
		Status: store.DeployAttemptStatusRunning, StartedAt: now,
		Snapshot: store.NewDeployAttemptSnapshot(svc),
	}); err != nil {
		logger.Error("deploy: record deploy attempt: save failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	if err := st.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		logger.Error("deploy: record deploy attempt: finish failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
	}
}

// PreviousKnownGoodImage returns the most recent successful deploy
// attempt's image for a service whose currentImage is currentImage,
// scanning attempts newest-first (ListDeployAttempts' own order) for the
// first successful attempt whose image actually differs. ok is false
// when no such attempt exists, either because this is the service's
// first-ever deploy or because it has already been rolled back to its
// oldest recorded image: the signal a caller (crashloop auto-rollback)
// uses to leave a crashloop to alert-only rather than rolling back to
// nothing.
func PreviousKnownGoodImage(attempts []store.DeployAttempt, currentImage string) (image string, ok bool) {
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		if a.Image == "" || a.Image == currentImage {
			continue
		}
		return a.Image, true
	}
	return "", false
}
