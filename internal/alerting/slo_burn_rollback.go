package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SLOAutoRollbackStore extends AutoRollbackStore (crashloop.go) with the
// save-approval surface store.AutoRollbackSLOBurnPauseForHuman mode
// needs. *store.DB satisfies this structurally.
type SLOAutoRollbackStore interface {
	AutoRollbackStore
	SaveDeployApproval(ctx context.Context, a store.DeployApproval) error
}

// SLOBurnHistoryRecorder is the narrow DB surface needed to log a dry-run would-have-rolled-back event.
type SLOBurnHistoryRecorder interface {
	RecordHistory(ctx context.Context, e HistoryEntry) error
}

// EventSLOBurnWouldRollback is the history event for a dry-run rollback that was evaluated but not applied.
const EventSLOBurnWouldRollback = "slo_burn_would_rollback"

// sloBurnRollbackActorType and sloBurnRollbackActor mark the system as requester of a pause_for_human approval.
const (
	sloBurnRollbackActorType = "system"
	sloBurnRollbackActor     = "alerting"
)

// defaultSLOBurnApprovalTTL sets the expiry of approvals opened here. The api expiry sweep acts on whatever ExpiresAt a row carries.
const defaultSLOBurnApprovalTTL = 24 * time.Hour

// MaybeAutoRollbackOnSLOBurn applies the app's SLO burn auto-rollback mode (auto, dry_run or pause_for_human) when a KindSLOBurn rule fires.
// Failures are logged, never returned, so one rule cannot block evaluation of the rest.
func MaybeAutoRollbackOnSLOBurn(ctx context.Context, st SLOAutoRollbackStore, history SLOBurnHistoryRecorder, nudger deploy.ReconcileNudger, tracker *AutoRollbackTracker, r Rule, notice string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	name, ok := domainHealthAppName(r.ResourceID)
	if !ok || name == "" {
		return
	}

	svc, err := st.GetDesiredService(ctx, name)
	if err != nil {
		logger.Error("alerting: slo burn auto-rollback: load app failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	mode := svc.AutoRollbackOnSLOBurn
	if mode == "" || mode == store.AutoRollbackSLOBurnOff {
		return
	}
	if tracker.alreadyHandled(r.ResourceID, svc.Image) {
		return
	}

	attempts, err := st.ListDeployAttempts(ctx, name)
	if err != nil {
		logger.Error("alerting: slo burn auto-rollback: list deploy attempts failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	// The image is tagged, and PruneDanglingImages only removes untagged images, so this rollback target stays available.
	image, ok := deploy.PreviousKnownGoodImage(attempts, svc.Image)
	if !ok {
		logger.Warn("alerting: slo burn auto-rollback: no older known-good image to fall back to, leaving SLO burn to alert only", slog.String("name", name), slog.String("image", svc.Image))
		return
	}

	switch mode {
	case store.AutoRollbackSLOBurnAuto:
		if _, err := deploy.TriggerImageDeploy(ctx, st, nudger, *svc, image, store.DeployAttemptSourceAutoRollback, logger); err != nil {
			logger.Error("alerting: slo burn auto-rollback: trigger deploy failed", slog.String("name", name), slog.String("image", image), slog.String("error", err.Error()))
			return
		}
		tracker.record(r.ResourceID, image)
		logger.Warn("alerting: slo burn auto-rollback: rolled back to prior image", slog.String("name", name), slog.String("from_image", svc.Image), slog.String("to_image", image))

	case store.AutoRollbackSLOBurnDryRun:
		recordSLOBurnDryRun(ctx, history, r, name, svc.Image, image, notice, logger)
		tracker.record(r.ResourceID, svc.Image)

	case store.AutoRollbackSLOBurnPauseForHuman:
		if err := requestSLOBurnRollbackApproval(ctx, st, r, *svc, image); err != nil {
			logger.Error("alerting: slo burn auto-rollback: create pending approval failed", slog.String("name", name), slog.String("image", image), slog.String("error", err.Error()))
			return
		}
		tracker.record(r.ResourceID, svc.Image)
		logger.Warn("alerting: slo burn auto-rollback: opened pending approval for rollback", slog.String("name", name), slog.String("from_image", svc.Image), slog.String("to_image", image))

	default:
		logger.Warn("alerting: slo burn auto-rollback: unknown mode, skipping", slog.String("name", name), slog.String("mode", mode))
	}
}

// recordSLOBurnDryRun logs a would-have-rolled-back event. Failures are logged, not propagated, so evaluation continues.
func recordSLOBurnDryRun(ctx context.Context, history SLOBurnHistoryRecorder, r Rule, app, fromImage, toImage, notice string, logger *slog.Logger) {
	if history == nil {
		return
	}
	detail := "dry run: would have rolled back from " + fromImage + " to " + toImage
	if notice != "" {
		detail += " (" + notice + ")"
	}
	if err := history.RecordHistory(ctx, HistoryEntry{
		RuleID: r.ID, RuleName: r.Name, RuleKind: string(r.Kind), ResourceID: r.ResourceID,
		App: app, Severity: severityOrDefault(r.Severity),
		Event: EventSLOBurnWouldRollback, Outcome: OutcomeSkipped, Detail: detail,
	}); err != nil {
		logger.Error("alerting: slo burn auto-rollback: record dry-run history failed", slog.String("error", err.Error()), slog.String("rule_id", r.ID))
	}
}

// requestSLOBurnRollbackApproval opens a pending DeployApproval for image, the same path a manual protected-environment deploy takes.
// Approving it runs svc's deploy through handleApproveDeployApproval.
// code path.
func requestSLOBurnRollbackApproval(ctx context.Context, st SLOAutoRollbackStore, r Rule, svc store.DesiredService, image string) error {
	id, err := store.NewDeployApprovalID()
	if err != nil {
		return fmt.Errorf("new deploy approval id: %w", err)
	}
	now := time.Now().UTC()
	return st.SaveDeployApproval(ctx, store.DeployApproval{
		ID: id, ServiceName: svc.Name, EnvironmentID: svc.EnvironmentID,
		Action: store.DeployApprovalActionDeploy, Image: image,
		Status:          store.DeployApprovalStatusPending,
		RequestedByType: sloBurnRollbackActorType, RequestedBy: sloBurnRollbackActor,
		RequestedByName: "SLO burn auto-rollback (" + r.Name + ")",
		CreatedAt:       store.FormatAuditTime(now),
		ExpiresAt:       store.FormatAuditTime(now.Add(defaultSLOBurnApprovalTTL)),
	})
}
