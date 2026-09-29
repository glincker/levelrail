package alerting

import (
	"context"
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

// SLOBurnHistoryRecorder is the narrow alerting.DB surface
// store.AutoRollbackSLOBurnDryRun mode needs to log a "would have rolled
// back" event, without threading NoiseControl's full history pipeline
// through this call. *DB satisfies this structurally.
type SLOBurnHistoryRecorder interface {
	RecordHistory(ctx context.Context, e HistoryEntry) error
}

// EventSLOBurnWouldRollback is dry-run mode's history event name,
// distinct from EventFired/EventResolved (a notification transition):
// this records that MaybeAutoRollbackOnSLOBurn evaluated a firing rule
// and would have rolled back in auto mode, without writing desired
// state.
const EventSLOBurnWouldRollback = "slo_burn_would_rollback"

// sloBurnRollbackActorType/sloBurnRollbackActor identify the requester of
// a pause_for_human approval opened by this file, the same "system," no-
// human-actor convention internal/api/app_events.go's appEventActorSystem
// already establishes for an automated action.
const (
	sloBurnRollbackActorType = "system"
	sloBurnRollbackActor     = "alerting"
)

// defaultSLOBurnApprovalTTL mirrors internal/api's own
// defaultDeployApprovalTTL (not imported: internal/api already depends on
// internal/alerting, so the reverse import would cycle). The api
// package's existing expiry sweep (RunDeployApprovalExpirySweep) acts on
// whatever ExpiresAt a row carries regardless of which package wrote it,
// so this only needs to be a sane value, not the same constant.
const defaultSLOBurnApprovalTTL = 24 * time.Hour

// MaybeAutoRollbackOnSLOBurn checks whether r's app has opted into SLO
// burn auto-rollback (store.DesiredService.AutoRollbackOnSLOBurn) and, if
// so, acts according to its configured mode:
//
//   - store.AutoRollbackSLOBurnAuto: rolls back immediately via
//     deploy.TriggerImageDeploy, the identical path MaybeAutoRollback
//     (crashloop.go) already uses for a crashloop.
//   - store.AutoRollbackSLOBurnDryRun: never deploys, only records a
//     "would have rolled back" history event.
//   - store.AutoRollbackSLOBurnPauseForHuman: never deploys directly,
//     opens a pending store.DeployApproval for the rollback image
//     instead, so a human approves it through the existing deploy-
//     approvals flow (internal/api/deploy_approvals.go) exactly like any
//     other gated deploy.
//
// Called both on a KindSLOBurn rule's pending-to-firing transition and on
// every tick it stays firing (Engine.Tick, mirroring MaybeAutoRollback's
// own becameFiring/stillFiring wiring there): tracker's alreadyHandled
// check is what prevents repeating the same action for the same incident
// in both cases. auto mode records the rollback *target* image (current
// desired state converges to it, the same dedup MaybeAutoRollback already
// relies on); dry_run and pause_for_human record the *current* (bad)
// image instead, since neither one changes desired state, so "nothing
// has changed since" is what "already handled" has to mean for them.
//
// Failures and "nothing to do" cases are logged, never returned: this
// runs as a side effect of Engine.Tick evaluating one rule among many,
// see MaybeAutoRollback's own doc comment for why that call must never
// block or fail evaluation of the rest.
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
	// image is always a tagged reference (deploy attempts never record an
	// untagged one), and internal/docker's own PruneDanglingImages only
	// ever removes untagged images, so this rollback target can never be
	// garbage-collected out from under it; see that function's own doc
	// comment.
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

// recordSLOBurnDryRun logs a would-have-rolled-back event to history. A
// nil history (SetSLOBurnAutoRollback never called) or a write failure is
// logged, not propagated: same "notification/log failure must not stop
// evaluation" reasoning MaybeAutoRollbackOnSLOBurn's own doc comment
// gives.
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

// requestSLOBurnRollbackApproval opens a pending store.DeployApproval for
// image, the same row shape and DeployApprovalActionDeploy path a manual
// deploy into a protected environment already goes through
// (requestDeployApproval, internal/api/deploy_approvals.go); approving it
// runs svc's deploy through the exact same handleApproveDeployApproval
// code path.
func requestSLOBurnRollbackApproval(ctx context.Context, st SLOAutoRollbackStore, r Rule, svc store.DesiredService, image string) error {
	id, err := store.NewDeployApprovalID()
	if err != nil {
		return err
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
