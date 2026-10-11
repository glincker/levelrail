package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrScheduledTaskRunNotFound is returned by GetScheduledTaskRun when id
// matches no run.
var ErrScheduledTaskRunNotFound = errors.New("store: scheduled task run not found")

// ScheduledTaskRun is one persisted job run (migrations/0467).
type ScheduledTaskRun struct {
	ID          string
	TaskID      string
	ServiceName string
	Trigger     string
	Status      string
	// ScheduledFor is the cron occurrence this run was claimed for, nil
	// for a manual or api run.
	ScheduledFor *time.Time
	StartedAt    time.Time
	FinishedAt   *time.Time
	ExitCode     *int
	DurationMS   int64
	Attempts     int
	NodeID       string
	Mode         string
	ContainerID  string
	Message      string
	Output       string
}

const scheduledTaskRunColumns = `id, task_id, service_name, trigger_kind, status, scheduled_for, started_at, finished_at, exit_code, duration_ms, attempts, node_id, mode, container_id, message, output`

// UpdateScheduledTaskConfig replaces every operator-editable field of a
// task. The schedule is re-armed (high-water mark cleared) when the cron
// expression or timezone changes, or the task goes from disabled to
// enabled, so editing a job never backfills occurrences it was not live for.
func (db *DB) UpdateScheduledTaskConfig(ctx context.Context, t ScheduledTask, now time.Time) error {
	cmdJSON, err := marshalCommand(t.Command)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", t.ID, err)
	}
	tz := orDefault(t.Timezone, "UTC")
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks SET
			armed_at = CASE WHEN (enabled = 0 AND ? = 1) OR schedule != ? OR timezone != ? THEN ? ELSE armed_at END,
			last_scheduled_for = CASE WHEN (enabled = 0 AND ? = 1) OR schedule != ? OR timezone != ? THEN NULL ELSE last_scheduled_for END,
			command = ?, schedule = ?, enabled = ?, concurrency_policy = ?, name = ?, mode = ?, timezone = ?,
			timeout_seconds = ?, max_retries = ?, retry_backoff_seconds = ?, catch_up = ?, catch_up_lookback_seconds = ?,
			cpu_limit_millis = ?, memory_limit_mb = ?, warn_after_seconds = ?, updated_at = ?
		WHERE id = ?
	`, t.Enabled, t.Schedule, tz, formatTime(now),
		t.Enabled, t.Schedule, tz,
		cmdJSON, t.Schedule, t.Enabled, normalizeConcurrencyPolicy(t.ConcurrencyPolicy), t.Name, orDefault(t.Mode, ScheduledTaskModeExec), tz,
		t.TimeoutSeconds, t.MaxRetries, t.RetryBackoffSeconds, orDefault(t.CatchUp, ScheduledTaskCatchUpSkip), t.CatchUpLookbackSeconds,
		t.CPULimitMillis, t.MemoryLimitMB, t.WarnAfterSeconds, formatTime(now), t.ID)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", t.ID, err)
	}
	return rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "update scheduled task %q", t.ID)
}

// CountScheduledTasksForService returns how many jobs an app has, for the
// per-app cap.
func (db *DB) CountScheduledTasksForService(ctx context.Context, serviceName string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduled_tasks WHERE service_name = ?`, serviceName).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count scheduled tasks for %q: %w", serviceName, err)
	}
	return n, nil
}

// ClaimScheduledTaskRun inserts run (with ScheduledFor set) and advances the
// task's high-water mark to that occurrence in one transaction. claimed is
// false when another claim for the same occurrence already exists; the mark
// is still advanced so the scheduler converges. A run in status running also
// sets running_since; a missed run bumps consecutive_missed.
func (db *DB) ClaimScheduledTaskRun(ctx context.Context, run ScheduledTaskRun) (claimed bool, err error) {
	if run.ScheduledFor == nil {
		return false, errors.New("store: claim scheduled task run: scheduled_for is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: claim scheduled task run %q: %w", run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO scheduled_task_runs (`+scheduledTaskRunColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, runArgs(run)...)
	if err != nil {
		return false, fmt.Errorf("store: claim scheduled task run %q: insert: %w", run.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: claim scheduled task run %q: rows affected: %w", run.ID, err)
	}
	claimed = n > 0

	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduled_tasks SET last_scheduled_for = ?
		WHERE id = ? AND (last_scheduled_for IS NULL OR last_scheduled_for < ?)
	`, formatTime(*run.ScheduledFor), run.TaskID, formatTime(*run.ScheduledFor)); err != nil {
		return false, fmt.Errorf("store: claim scheduled task run %q: advance mark: %w", run.ID, err)
	}
	if claimed {
		if err := applyRunStartedTx(ctx, tx, run); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: claim scheduled task run %q: commit: %w", run.ID, err)
	}
	return claimed, nil
}

// CreateScheduledTaskRun inserts a manual or api run (no occurrence claim).
func (db *DB) CreateScheduledTaskRun(ctx context.Context, run ScheduledTaskRun) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create scheduled task run %q: %w", run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scheduled_task_runs (`+scheduledTaskRunColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, runArgs(run)...); err != nil {
		return fmt.Errorf("store: create scheduled task run %q: %w", run.ID, err)
	}
	if err := applyRunStartedTx(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: create scheduled task run %q: commit: %w", run.ID, err)
	}
	return nil
}

func applyRunStartedTx(ctx context.Context, tx *sql.Tx, run ScheduledTaskRun) error {
	switch run.Status {
	case ScheduledTaskStatusRunning:
		if _, err := tx.ExecContext(ctx, `UPDATE scheduled_tasks SET running_since = COALESCE(running_since, ?) WHERE id = ?`,
			formatTime(run.StartedAt), run.TaskID); err != nil {
			return fmt.Errorf("store: mark scheduled task %q running: %w", run.TaskID, err)
		}
	case ScheduledTaskStatusMissed:
		if _, err := tx.ExecContext(ctx, `UPDATE scheduled_tasks SET consecutive_missed = consecutive_missed + 1 WHERE id = ?`, run.TaskID); err != nil {
			return fmt.Errorf("store: count missed run for %q: %w", run.TaskID, err)
		}
	}
	return nil
}

// FinishScheduledTaskRun writes a run's terminal state and clears the
// task's running_since when no other run of it is still in flight. A
// success resets consecutive_missed.
func (db *DB) FinishScheduledTaskRun(ctx context.Context, run ScheduledTaskRun) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: %w", run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	var exit sql.NullInt64
	if run.ExitCode != nil {
		exit = sql.NullInt64{Int64: int64(*run.ExitCode), Valid: true}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE scheduled_task_runs SET status = ?, finished_at = ?, exit_code = ?, duration_ms = ?, attempts = ?,
			node_id = ?, mode = ?, container_id = ?, message = ?, output = ?
		WHERE id = ?
	`, run.Status, formatTimePtr(run.FinishedAt), exit, run.DurationMS, run.Attempts, run.NodeID, run.Mode, run.ContainerID, run.Message, run.Output, run.ID)
	if err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: %w", run.ID, err)
	}
	if err := rowsAffectedOrNotFound(res, ErrScheduledTaskRunNotFound, "finish scheduled task run %q", run.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduled_tasks SET
			running_since = CASE WHEN EXISTS (SELECT 1 FROM scheduled_task_runs WHERE task_id = ? AND status = ?) THEN running_since ELSE NULL END,
			consecutive_missed = CASE WHEN ? = ? THEN 0 ELSE consecutive_missed END
		WHERE id = ?
	`, run.TaskID, ScheduledTaskStatusRunning, run.Status, ScheduledTaskStatusSuccess, run.TaskID); err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: update task: %w", run.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: commit: %w", run.ID, err)
	}
	return nil
}

// InterruptRunningScheduledTaskRuns marks every run still in status running
// as interrupted. Called once at control-plane start, when no run can
// legitimately be in flight; the runs are returned so their containers can
// be cleaned up. Interrupted runs are never retried, which is what rules
// out a double run after a restart.
func (db *DB) InterruptRunningScheduledTaskRuns(ctx context.Context, now time.Time) ([]ScheduledTaskRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+scheduledTaskRunColumns+` FROM scheduled_task_runs WHERE status = ?`, ScheduledTaskStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("store: list running scheduled task runs: %w", err)
	}
	runs, err := scanScheduledTaskRuns(rows)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		r := &runs[i]
		fin := now
		r.Status, r.FinishedAt = ScheduledTaskStatusInterrupted, &fin
		r.DurationMS = now.Sub(r.StartedAt).Milliseconds()
		r.Message = "control plane restarted while this run was in flight; it was not retried"
		if _, err := db.ExecContext(ctx, `UPDATE scheduled_task_runs SET status = ?, finished_at = ?, duration_ms = ?, message = ? WHERE id = ?`,
			r.Status, formatTime(fin), r.DurationMS, r.Message, r.ID); err != nil {
			return nil, fmt.Errorf("store: interrupt scheduled task run %q: %w", r.ID, err)
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE scheduled_tasks SET running_since = NULL, last_run_at = ?, last_run_status = ?, last_run_output = ?,
				consecutive_failures = consecutive_failures + 1
			WHERE id = ?`, formatTime(now), ScheduledTaskStatusInterrupted, r.Message, r.TaskID); err != nil {
			return nil, fmt.Errorf("store: interrupt scheduled task %q: %w", r.TaskID, err)
		}
	}
	return runs, nil
}

// GetScheduledTaskRun returns one run including its output.
func (db *DB) GetScheduledTaskRun(ctx context.Context, id string) (ScheduledTaskRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+scheduledTaskRunColumns+` FROM scheduled_task_runs WHERE id = ?`, id)
	if err != nil {
		return ScheduledTaskRun{}, fmt.Errorf("store: get scheduled task run %q: %w", id, err)
	}
	runs, err := scanScheduledTaskRuns(rows)
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	if len(runs) == 0 {
		return ScheduledTaskRun{}, ErrScheduledTaskRunNotFound
	}
	return runs[0], nil
}

// ScheduledTaskRunFilter narrows ListScheduledTaskRuns.
type ScheduledTaskRunFilter struct {
	ServiceName string
	TaskID      string
	Status      string
	Limit       int
}

// ListScheduledTaskRuns returns runs newest first without their output
// (GetScheduledTaskRun carries it), so a history table stays light.
func (db *DB) ListScheduledTaskRuns(ctx context.Context, f ScheduledTaskRunFilter) ([]ScheduledTaskRun, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, task_id, service_name, trigger_kind, status, scheduled_for, started_at, finished_at, exit_code, duration_ms, attempts, node_id, mode, container_id, message, '' FROM scheduled_task_runs WHERE service_name = ?`
	args := []any{f.ServiceName}
	if f.TaskID != "" {
		q += ` AND task_id = ?`
		args = append(args, f.TaskID)
	}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled task runs for %q: %w", f.ServiceName, err)
	}
	return scanScheduledTaskRuns(rows)
}

// PruneScheduledTaskRuns keeps the newest keep runs of a task and blanks the
// output of runs started before outputCutoff (zero skips the output pass).
func (db *DB) PruneScheduledTaskRuns(ctx context.Context, taskID string, keep int, outputCutoff time.Time) error {
	if keep > 0 {
		if _, err := db.ExecContext(ctx, `
			DELETE FROM scheduled_task_runs WHERE task_id = ? AND status != ? AND id NOT IN (
				SELECT id FROM scheduled_task_runs WHERE task_id = ? ORDER BY started_at DESC, id DESC LIMIT ?)
		`, taskID, ScheduledTaskStatusRunning, taskID, keep); err != nil {
			return fmt.Errorf("store: prune scheduled task runs for %q: %w", taskID, err)
		}
	}
	if !outputCutoff.IsZero() {
		if _, err := db.ExecContext(ctx, `UPDATE scheduled_task_runs SET output = '' WHERE task_id = ? AND output != '' AND started_at < ?`,
			taskID, formatTime(outputCutoff)); err != nil {
			return fmt.Errorf("store: prune scheduled task run output for %q: %w", taskID, err)
		}
	}
	return nil
}

// SyncSpecScheduledTasks makes the spec-sourced jobs of a service match
// jobs (keyed by Name): new ones are inserted, existing ones updated in
// place (keeping ID and history), and spec-sourced jobs no longer listed
// are deleted. Jobs created through the API are never touched.
func (db *DB) SyncSpecScheduledTasks(ctx context.Context, serviceName string, jobs []ScheduledTask, now time.Time, newID func() (string, error)) error {
	existing, err := db.ListScheduledTasksForService(ctx, serviceName)
	if err != nil {
		return err
	}
	byName := make(map[string]ScheduledTask, len(existing))
	for _, e := range existing {
		if e.Name != "" {
			byName[e.Name] = e
		}
	}
	keep := make(map[string]struct{}, len(jobs))
	for _, j := range jobs {
		keep[j.Name] = struct{}{}
		j.ServiceName, j.Source = serviceName, ScheduledTaskSourceSpec
		if cur, ok := byName[j.Name]; ok {
			if cur.Source != ScheduledTaskSourceSpec {
				return fmt.Errorf("store: job %q on %q already exists and is not managed by the spec", j.Name, serviceName)
			}
			j.ID = cur.ID
			if err := db.UpdateScheduledTaskConfig(ctx, j, now); err != nil {
				return err
			}
			continue
		}
		id, err := newID()
		if err != nil {
			return fmt.Errorf("store: sync spec jobs for %q: %w", serviceName, err)
		}
		j.ID, j.CreatedAt, j.UpdatedAt = id, now, now
		if err := db.SaveScheduledTask(ctx, j); err != nil {
			return err
		}
	}
	for _, e := range existing {
		if e.Source != ScheduledTaskSourceSpec {
			continue
		}
		if _, ok := keep[e.Name]; ok {
			continue
		}
		if err := db.DeleteScheduledTask(ctx, e.ID); err != nil && !errors.Is(err, ErrScheduledTaskNotFound) {
			return err
		}
	}
	return nil
}

func runArgs(r ScheduledTaskRun) []any {
	var exit sql.NullInt64
	if r.ExitCode != nil {
		exit = sql.NullInt64{Int64: int64(*r.ExitCode), Valid: true}
	}
	return []any{r.ID, r.TaskID, r.ServiceName, r.Trigger, r.Status, formatTimePtr(r.ScheduledFor), formatTime(r.StartedAt),
		formatTimePtr(r.FinishedAt), exit, r.DurationMS, r.Attempts, r.NodeID, r.Mode, r.ContainerID, r.Message, r.Output}
}

func scanScheduledTaskRuns(rows *sql.Rows) ([]ScheduledTaskRun, error) {
	defer func() { _ = rows.Close() }()
	var out []ScheduledTaskRun
	for rows.Next() {
		var (
			r                        ScheduledTaskRun
			scheduledFor, finishedAt sql.NullString
			startedAt                string
			exit                     sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.TaskID, &r.ServiceName, &r.Trigger, &r.Status, &scheduledFor, &startedAt, &finishedAt, &exit,
			&r.DurationMS, &r.Attempts, &r.NodeID, &r.Mode, &r.ContainerID, &r.Message, &r.Output); err != nil {
			return nil, fmt.Errorf("store: scan scheduled task run: %w", err)
		}
		var err error
		if r.ScheduledFor, err = parseTimePtr(scheduledFor); err != nil {
			return nil, fmt.Errorf("store: parse scheduled_for: %w", err)
		}
		if r.FinishedAt, err = parseTimePtr(finishedAt); err != nil {
			return nil, fmt.Errorf("store: parse finished_at: %w", err)
		}
		if r.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
			return nil, fmt.Errorf("store: parse started_at: %w", err)
		}
		if exit.Valid {
			v := int(exit.Int64)
			r.ExitCode = &v
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled task runs: %w", err)
	}
	return out, nil
}
