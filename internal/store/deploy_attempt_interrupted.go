package store

import (
	"context"
	"fmt"
	"time"

	"github.com/glincker/levelrail/internal/secretmask"
)

// Reasons recorded on attempts that a restart or a lost build left behind.
const (
	DeployReasonInterrupted = "interrupted: the control plane restarted while this deploy was running"
	DeployReasonOrphaned    = "orphaned: no build is running for this deploy"
	// DeployReasonConverging marks an attempt that had already written desired
	// state when the process died; the reconciler finishes its rollout.
	DeployReasonConverging = "recovered: the control plane restarted after this deploy cut over; the rollout continues"
)

// Outcomes of RecoverInterruptedDeployAttempts for one attempt.
const (
	RecoveredInterrupted = "interrupted"
	RecoveredConverging  = "converging"
	RecoveredRequeued    = "requeued"
)

// RecoveredDeploy is one attempt RecoverInterruptedDeployAttempts changed.
type RecoveredDeploy struct {
	ID          string
	ServiceName string
	Image       string
	Outcome     string
}

// RecoverInterruptedDeployAttempts settles every attempt a previous process
// left running. Run once at boot before the queue drains: a running row then
// has no owner. Queued attempts are untouched and run again. Safe to repeat.
func (db *DB) RecoverInterruptedDeployAttempts(ctx context.Context, now time.Time) ([]RecoveredDeploy, error) {
	running, err := db.ListRunningDeployAttempts(ctx)
	if err != nil {
		return nil, err
	}
	var out []RecoveredDeploy
	for _, a := range running {
		outcome, moved, err := db.recoverOne(ctx, a, now)
		if err != nil {
			return out, err
		}
		if moved {
			out = append(out, RecoveredDeploy{ID: a.ID, ServiceName: a.ServiceName, Image: a.Image, Outcome: outcome})
		}
	}
	return out, nil
}

func (db *DB) recoverOne(ctx context.Context, a DeployAttempt, now time.Time) (string, bool, error) {
	if a.HeldRequest != "" {
		// A released held deploy goes back to held so the releaser replays it.
		moved, err := db.TransitionDeployAttempt(ctx, a.ID, DeployAttemptStatusRunning, DeployAttemptStatusHeld, DeployReasonFrozen+": release interrupted by restart")
		return RecoveredRequeued, moved, err
	}
	// Desired state naming this attempt's image means it cut over before the
	// crash: failing it would contradict what the reconciler now serves.
	var one int
	cut := a.Image != "" && db.QueryRowContext(ctx, `SELECT 1 FROM desired_services WHERE name = ? AND image = ?`, a.ServiceName, a.Image).Scan(&one) == nil
	if cut {
		moved, err := db.TransitionDeployAttempt(ctx, a.ID, DeployAttemptStatusRunning, DeployAttemptStatusSucceeded, DeployReasonConverging)
		return RecoveredConverging, moved, err
	}
	moved, err := db.FinishRunningDeployAttempt(ctx, a.ID, DeployAttemptStatusFailed, now, DeployReasonInterrupted)
	if err != nil || !moved {
		return RecoveredInterrupted, false, err
	}
	return RecoveredInterrupted, true, db.SetDeployAttemptReason(ctx, a.ID, DeployReasonInterrupted)
}

// CancelOrphanedDeployAttempt finishes a running attempt no build owns as
// canceled with the orphaned reason. Reports whether it moved.
func (db *DB) CancelOrphanedDeployAttempt(ctx context.Context, id, by string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, reason = ?, canceled_by = ?, finished_at = ?
		WHERE id = ? AND status = ?
	`, DeployAttemptStatusCanceled, secretmask.Mask(DeployReasonOrphaned), secretmask.Mask(by), now.UTC().Format(time.RFC3339Nano), id, DeployAttemptStatusRunning)
	if err != nil {
		return false, fmt.Errorf("store: cancel orphaned deploy attempt %q: %w", id, err)
	}
	return movedRows(res, nil, "cancel orphaned deploy attempt", id)
}
