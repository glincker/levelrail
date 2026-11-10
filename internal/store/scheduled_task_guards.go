package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// guardTimeFormat is fixed width, so marks in the new columns compare as text.
const guardTimeFormat = "2006-01-02T15:04:05.000000000Z"

func formatGuardTime(t time.Time) string { return t.UTC().Format(guardTimeFormat) }

// ScheduledTaskConcurrencySkip is the overlap name for forbid; the API
// stores it as forbid. ScheduledTaskConcurrencyQueueOne keeps at most one
// waiting occurrence behind a running one and skips the rest.
const (
	ScheduledTaskConcurrencySkip     = "skip"
	ScheduledTaskConcurrencyQueueOne = "queue-one"
	// ScheduledTaskStatusQueued is a queue-one run waiting for the running one.
	ScheduledTaskStatusQueued = "queued"
)

// ScheduledTaskGuards are the per-job guard rails outside the spec-managed
// config: KeepRuns and KeepDays use 0 as "the control plane default".
type ScheduledTaskGuards struct {
	AlertOnFailure bool
	KeepRuns       int
	KeepDays       int
}

// ClaimOutcome says what a guarded claim did with one cron occurrence.
type ClaimOutcome string

// Guarded claim outcomes.
const (
	ClaimStarted   ClaimOutcome = "started"
	ClaimSkipped   ClaimOutcome = "skipped"
	ClaimQueued    ClaimOutcome = "queued"
	ClaimDuplicate ClaimOutcome = "duplicate"
)

// GetScheduledTaskGuards reads a job's guard rails.
func (db *DB) GetScheduledTaskGuards(ctx context.Context, id string) (ScheduledTaskGuards, error) {
	var g ScheduledTaskGuards
	var alert int
	err := db.QueryRowContext(ctx, `SELECT alert_on_failure, keep_runs, keep_days FROM scheduled_tasks WHERE id = ?`, id).
		Scan(&alert, &g.KeepRuns, &g.KeepDays)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrScheduledTaskNotFound
	}
	if err != nil {
		return g, fmt.Errorf("store: get scheduled task guards %q: %w", id, err)
	}
	g.AlertOnFailure = alert != 0
	return g, nil
}

// SetScheduledTaskGuards replaces a job's guard rails. Turning the alert
// off clears its rate limit marks so a later opt-in alerts on the next failure.
func (db *DB) SetScheduledTaskGuards(ctx context.Context, id string, g ScheduledTaskGuards) error {
	if g.KeepRuns < 0 || g.KeepDays < 0 {
		return errors.New("store: scheduled task guards must not be negative")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set scheduled task guards %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE scheduled_tasks SET alert_on_failure = ?, keep_runs = ?, keep_days = ? WHERE id = ?`,
		g.AlertOnFailure, g.KeepRuns, g.KeepDays, id)
	if err != nil {
		return fmt.Errorf("store: set scheduled task guards %q: %w", id, err)
	}
	if err := rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "set scheduled task guards %q", id); err != nil {
		return err
	}
	if !g.AlertOnFailure {
		if _, err := tx.ExecContext(ctx, `DELETE FROM job_failure_alerts WHERE task_id = ?`, id); err != nil {
			return fmt.Errorf("store: clear job failure alert marks %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set scheduled task guards %q: commit: %w", id, err)
	}
	return nil
}

// ClaimGuardedScheduledTaskRun claims one occurrence like ClaimScheduledTaskRun,
// but for skip (forbid) and queue-one the start is a single conditional INSERT
// in the claim transaction, so two schedulers on one store never both start.
func (db *DB) ClaimGuardedScheduledTaskRun(ctx context.Context, run ScheduledTaskRun) (ClaimOutcome, error) {
	if run.ScheduledFor == nil {
		return "", errors.New("store: guarded claim: scheduled_for is required")
	}
	var policy string
	if err := db.QueryRowContext(ctx, `SELECT concurrency_policy FROM scheduled_tasks WHERE id = ?`, run.TaskID).Scan(&policy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ClaimDuplicate, nil
		}
		return "", fmt.Errorf("store: guarded claim %q: load policy: %w", run.ID, err)
	}
	if run.Status != ScheduledTaskStatusRunning || (policy != ScheduledTaskConcurrencyForbid && policy != ScheduledTaskConcurrencyQueueOne) {
		claimed, err := db.ClaimScheduledTaskRun(ctx, run)
		if err != nil || !claimed {
			return ClaimDuplicate, err
		}
		return ClaimStarted, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: guarded claim %q: %w", run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	outcome, err := guardedInsertTx(ctx, tx, run, policy)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduled_tasks SET last_scheduled_for = ?
		WHERE id = ? AND (last_scheduled_for IS NULL OR last_scheduled_for < ?)
	`, formatTime(*run.ScheduledFor), run.TaskID, formatTime(*run.ScheduledFor)); err != nil {
		return "", fmt.Errorf("store: guarded claim %q: advance mark: %w", run.ID, err)
	}
	if outcome == ClaimStarted {
		if err := applyRunStartedTx(ctx, tx, run); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: guarded claim %q: commit: %w", run.ID, err)
	}
	return outcome, nil
}

// Guarded inserts are whole constant statements; only values are bound.
const (
	insertRunSelect  = `INSERT INTO scheduled_task_runs (` + scheduledTaskRunColumns + `) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE `
	insertRunIfIdle  = insertRunSelect + `NOT EXISTS (SELECT 1 FROM scheduled_task_runs b WHERE b.task_id = ? AND b.status IN ('running', 'queued')) ON CONFLICT DO NOTHING`
	insertRunIfNoneQ = insertRunSelect + `NOT EXISTS (SELECT 1 FROM scheduled_task_runs q WHERE q.task_id = ? AND q.status = 'queued') ON CONFLICT DO NOTHING`
	insertRunAlways  = insertRunSelect + `1 = 1 ON CONFLICT DO NOTHING`
)

func guardedInsertTx(ctx context.Context, tx *sql.Tx, run ScheduledTaskRun, policy string) (ClaimOutcome, error) {
	var dup int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduled_task_runs WHERE task_id = ? AND scheduled_for = ?`,
		run.TaskID, formatTime(*run.ScheduledFor)).Scan(&dup); err != nil {
		return "", fmt.Errorf("store: guarded claim %q: duplicate check: %w", run.ID, err)
	}
	if dup > 0 {
		return ClaimDuplicate, nil
	}
	insert := func(r ScheduledTaskRun, query string, condArgs ...any) (bool, error) {
		args := append(runArgs(r), condArgs...)
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return false, fmt.Errorf("store: guarded claim %q: insert: %w", r.ID, err)
		}
		n, err := res.RowsAffected()
		return n > 0, err
	}
	if ok, err := insert(run, insertRunIfIdle, run.TaskID); err != nil || ok {
		return ClaimStarted, err
	}
	if policy == ScheduledTaskConcurrencyQueueOne {
		queued := run
		queued.Status = ScheduledTaskStatusQueued
		ok, err := insert(queued, insertRunIfNoneQ, run.TaskID)
		if err != nil || ok {
			return ClaimQueued, err
		}
	}
	skipped := run
	fin := run.StartedAt
	skipped.Status, skipped.FinishedAt = ScheduledTaskStatusSkippedConcurrency, &fin
	skipped.Message = "skipped: a previous run of this job is still in flight (overlap policy " + policy + ")"
	if _, err := insert(skipped, insertRunAlways); err != nil {
		return "", err
	}
	return ClaimSkipped, nil
}

// QueuedTarget is one enabled job the caller will launch a promoted run for.
// MaxWait expires a queued run older than it instead of starting it late.
type QueuedTarget struct {
	TaskID  string
	MaxWait time.Duration
}

// PromoteQueuedScheduledTaskRuns starts the queued run of each target whose
// previous run finished. Only the caller's targets are touched, so every
// promoted row has a caller that runs it; each promotion is one conditional UPDATE.
func (db *DB) PromoteQueuedScheduledTaskRuns(ctx context.Context, now time.Time, targets []QueuedTarget) ([]ScheduledTaskRun, error) {
	var out []ScheduledTaskRun
	for _, tg := range targets {
		var id, started string
		err := db.QueryRowContext(ctx, `SELECT id, started_at FROM scheduled_task_runs WHERE task_id = ? AND status = 'queued' LIMIT 1`, tg.TaskID).Scan(&id, &started)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, fmt.Errorf("store: load queued run for %q: %w", tg.TaskID, err)
		}
		if at, perr := time.Parse(time.RFC3339Nano, started); perr == nil && tg.MaxWait > 0 && now.Sub(at) > tg.MaxWait {
			if _, err := db.ExecContext(ctx, `UPDATE scheduled_task_runs SET status = ?, finished_at = ?, message = ? WHERE id = ? AND status = 'queued'`,
				ScheduledTaskStatusMissed, formatTime(now), "queued run expired before the previous run finished", id); err != nil {
				return out, fmt.Errorf("store: expire queued run %q: %w", id, err)
			}
			continue
		}
		run, ok, err := db.promoteOne(ctx, id, now)
		if err != nil {
			return out, err
		}
		if ok {
			out = append(out, run)
		}
	}
	return out, nil
}

func (db *DB) promoteOne(ctx context.Context, id string, now time.Time) (ScheduledTaskRun, bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ScheduledTaskRun{}, false, fmt.Errorf("store: promote queued run %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE scheduled_task_runs SET status = 'running', started_at = ?
		WHERE id = ? AND status = 'queued'
			AND EXISTS (SELECT 1 FROM scheduled_tasks t WHERE t.id = scheduled_task_runs.task_id AND t.enabled = 1)
			AND NOT EXISTS (SELECT 1 FROM scheduled_task_runs r WHERE r.task_id = scheduled_task_runs.task_id AND r.status = 'running')`,
		formatTime(now), id)
	if err != nil {
		return ScheduledTaskRun{}, false, fmt.Errorf("store: promote queued run %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ScheduledTaskRun{}, false, nil
	}
	runs, err := scanScheduledTaskRunsTx(ctx, tx, id)
	if err != nil || len(runs) == 0 {
		return ScheduledTaskRun{}, false, err
	}
	if err := applyRunStartedTx(ctx, tx, runs[0]); err != nil {
		return ScheduledTaskRun{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ScheduledTaskRun{}, false, fmt.Errorf("store: promote queued run %q: commit: %w", id, err)
	}
	return runs[0], true, nil
}

func scanScheduledTaskRunsTx(ctx context.Context, tx *sql.Tx, id string) ([]ScheduledTaskRun, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+scheduledTaskRunColumns+` FROM scheduled_task_runs WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: load promoted run %q: %w", id, err)
	}
	return scanScheduledTaskRuns(rows)
}

// Prune deletes pick runs oldest first, bounded by the trailing LIMIT. Skipped
// runs have their own cap so a busy overlap never pushes out real history.
const (
	pruneSkipped = `DELETE FROM scheduled_task_runs WHERE id IN (SELECT id FROM scheduled_task_runs WHERE task_id = ? AND status = 'skipped_concurrency'
		AND id NOT IN (SELECT id FROM scheduled_task_runs WHERE task_id = ? AND status = 'skipped_concurrency' ORDER BY started_at DESC, id DESC LIMIT ?)
		ORDER BY started_at ASC, id ASC LIMIT ?)`
	pruneBeyondKeep = `DELETE FROM scheduled_task_runs WHERE id IN (SELECT id FROM scheduled_task_runs WHERE task_id = ? AND status NOT IN ('running', 'queued', 'skipped_concurrency')
		AND id NOT IN (SELECT id FROM scheduled_task_runs WHERE task_id = ? AND status NOT IN ('running', 'queued', 'skipped_concurrency') ORDER BY started_at DESC, id DESC LIMIT ?)
		ORDER BY started_at ASC, id ASC LIMIT ?)`
	pruneOlderThan = `DELETE FROM scheduled_task_runs WHERE id IN (SELECT id FROM scheduled_task_runs WHERE task_id = ? AND status NOT IN ('running', 'queued')
		AND started_at < ? ORDER BY started_at ASC, id ASC LIMIT ?)`
)

// RunRetention bounds one prune pass of a job's history. MaxRows caps the
// rows deleted per pass; 0 deletes nothing, so a prune is never unbounded.
type RunRetention struct {
	Keep         int
	SkippedKeep  int
	Before       time.Time
	OutputCutoff time.Time
	MaxRows      int
}

// PruneScheduledTaskRuns deletes, oldest first and at most p.MaxRows rows,
// skipped runs beyond p.SkippedKeep, executed runs beyond p.Keep and runs
// started before p.Before, then blanks old output. Running and queued runs stay.
func (db *DB) PruneScheduledTaskRuns(ctx context.Context, taskID string, p RunRetention) error {
	budget := p.MaxRows
	del := func(query string, args ...any) error {
		if budget <= 0 {
			return nil
		}
		res, err := db.ExecContext(ctx, query, append(args, budget)...)
		if err != nil {
			return fmt.Errorf("store: prune scheduled task runs for %q: %w", taskID, err)
		}
		n, _ := res.RowsAffected()
		budget -= int(n)
		return nil
	}
	if p.SkippedKeep > 0 {
		if err := del(pruneSkipped, taskID, taskID, p.SkippedKeep); err != nil {
			return err
		}
	}
	if p.Keep > 0 {
		if err := del(pruneBeyondKeep, taskID, taskID, p.Keep); err != nil {
			return err
		}
	}
	if !p.Before.IsZero() {
		if err := del(pruneOlderThan, taskID, formatTime(p.Before)); err != nil {
			return err
		}
	}
	if !p.OutputCutoff.IsZero() {
		if _, err := db.ExecContext(ctx, `UPDATE scheduled_task_runs SET output = '' WHERE task_id = ? AND output != '' AND started_at < ?`,
			taskID, formatTime(p.OutputCutoff)); err != nil {
			return fmt.Errorf("store: prune scheduled task run output for %q: %w", taskID, err)
		}
	}
	return nil
}

// JobFailureAlert is one job whose failure alert slot was just claimed. It
// carries identifiers and the exit code only, never run output or env.
type JobFailureAlert struct {
	TaskID      string
	ServiceName string
	Name        string
	Status      string
	ExitCode    *int
}

// A mark is due when it is older than the window and the job ran again since.
const (
	selectDueJobFailures = `SELECT t.id, t.service_name, t.name, t.last_run_status, t.last_run_at FROM scheduled_tasks t
		LEFT JOIN job_failure_alerts a ON a.rule_id = ? AND a.task_id = t.id
		WHERE t.service_name = ? AND t.alert_on_failure = 1 AND t.consecutive_failures > 0 AND t.last_run_at IS NOT NULL
			AND t.last_run_status IN ('failed', 'timeout', 'container_not_running')
			AND (a.task_id IS NULL OR (a.last_alert_at < ? AND a.alerted_run_at != t.last_run_at))
		ORDER BY t.id LIMIT ?`
	claimJobFailure = `INSERT INTO job_failure_alerts (rule_id, task_id, last_alert_at, alerted_run_at, prev_alert_at, prev_run_at)
		VALUES (?, ?, ?, ?, NULL, NULL)
		ON CONFLICT (rule_id, task_id) DO UPDATE SET prev_alert_at = job_failure_alerts.last_alert_at, prev_run_at = job_failure_alerts.alerted_run_at,
			last_alert_at = excluded.last_alert_at, alerted_run_at = excluded.alerted_run_at
		WHERE job_failure_alerts.last_alert_at < ? AND job_failure_alerts.alerted_run_at != excluded.alerted_run_at`
)

// ClaimJobFailureAlerts marks and returns, at most limit, failing jobs of
// service that opted in and are due an alert for ruleID. The mark is per rule
// and job, and each claim is one conditional upsert, so two engines never both send.
func (db *DB) ClaimJobFailureAlerts(ctx context.Context, ruleID, service string, now, windowStart time.Time, limit int) ([]JobFailureAlert, error) {
	if limit <= 0 || ruleID == "" || service == "" {
		return nil, nil
	}
	type cand struct {
		a     JobFailureAlert
		runAt string
	}
	rows, err := db.QueryContext(ctx, selectDueJobFailures, ruleID, service, formatGuardTime(windowStart), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list due job failure alerts: %w", err)
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.a.TaskID, &c.a.ServiceName, &c.a.Name, &c.a.Status, &c.runAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan job failure alert: %w", err)
		}
		cands = append(cands, c)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list due job failure alerts: %w", err)
	}
	var out []JobFailureAlert
	for _, c := range cands {
		res, err := db.ExecContext(ctx, claimJobFailure, ruleID, c.a.TaskID, formatGuardTime(now), c.runAt, formatGuardTime(windowStart))
		if err != nil {
			return out, fmt.Errorf("store: claim job failure alert %q: %w", c.a.TaskID, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		var code sql.NullInt64
		err = db.QueryRowContext(ctx, `SELECT exit_code FROM scheduled_task_runs WHERE task_id = ? AND status IN (?, ?, ?)
			ORDER BY started_at DESC, id DESC LIMIT 1`, c.a.TaskID, ScheduledTaskStatusSuccess, ScheduledTaskStatusFailed, ScheduledTaskStatusTimeout).Scan(&code)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, fmt.Errorf("store: job failure alert exit code %q: %w", c.a.TaskID, err)
		}
		if code.Valid {
			v := int(code.Int64)
			c.a.ExitCode = &v
		}
		out = append(out, c.a)
	}
	return out, nil
}

// ReleaseJobFailureAlert undoes a claim made at claimedAt whose notice was
// not delivered, so a broken channel does not silence the job for the window.
func (db *DB) ReleaseJobFailureAlert(ctx context.Context, ruleID, taskID string, claimedAt time.Time) error {
	at := formatGuardTime(claimedAt)
	if _, err := db.ExecContext(ctx, `DELETE FROM job_failure_alerts WHERE rule_id = ? AND task_id = ? AND last_alert_at = ? AND prev_alert_at IS NULL`,
		ruleID, taskID, at); err != nil {
		return fmt.Errorf("store: release job failure alert %q: %w", taskID, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE job_failure_alerts SET last_alert_at = prev_alert_at, alerted_run_at = prev_run_at, prev_alert_at = NULL, prev_run_at = NULL
		WHERE rule_id = ? AND task_id = ? AND last_alert_at = ?`, ruleID, taskID, at); err != nil {
		return fmt.Errorf("store: release job failure alert %q: %w", taskID, err)
	}
	return nil
}
