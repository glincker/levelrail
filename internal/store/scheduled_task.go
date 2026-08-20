package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Scheduled task run statuses migrations/0047_scheduled_tasks.sql's CHECK
// constraint accepts, mirroring backup_history's own BackupStatus*
// constants exactly.
const (
	ScheduledTaskRunStatusRunning   = "running"
	ScheduledTaskRunStatusSucceeded = "succeeded"
	ScheduledTaskRunStatusFailed    = "failed"
)

// ErrScheduledTaskNotFound is returned by GetScheduledTask,
// UpdateScheduledTask, and DeleteScheduledTask when id doesn't match any
// row.
var ErrScheduledTaskNotFound = errors.New("store: scheduled task not found")

// ErrScheduledTaskRunNotFound is returned by FinishScheduledTaskRun when
// id doesn't match any row.
var ErrScheduledTaskRunNotFound = errors.New("store: scheduled task run not found")

// ScheduledTask is an arbitrary command run inside one service's
// container on a cron schedule (internal/scheduledtask.Scheduler).
// Command is always executed as `sh -c Command`, see
// migrations/0047_scheduled_tasks.sql's own doc comment for why.
type ScheduledTask struct {
	ID             string
	ServiceName    string
	Name           string
	Command        string
	Schedule       string
	Enabled        bool
	TimeoutSeconds int
	CreatedAt      string
	UpdatedAt      string
}

// SaveScheduledTask inserts a new scheduled task row. ID is minted by the
// caller (internal/api), the same "generate before the INSERT" pattern
// store.BackupTarget's own doc comment describes.
func (db *DB) SaveScheduledTask(ctx context.Context, t ScheduledTask) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (id, service_name, name, command, schedule, enabled, timeout_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.ServiceName, t.Name, t.Command, t.Schedule, t.Enabled, t.TimeoutSeconds, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save scheduled task %q: %w", t.ID, err)
	}
	return nil
}

// GetScheduledTask returns the scheduled task with this ID, or
// ErrScheduledTaskNotFound.
func (db *DB) GetScheduledTask(ctx context.Context, id string) (ScheduledTask, error) {
	var t ScheduledTask
	err := db.QueryRowContext(ctx, `
		SELECT id, service_name, name, command, schedule, enabled, timeout_seconds, created_at, updated_at
		FROM scheduled_tasks
		WHERE id = ?
	`, id).Scan(&t.ID, &t.ServiceName, &t.Name, &t.Command, &t.Schedule, &t.Enabled, &t.TimeoutSeconds, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTask{}, ErrScheduledTaskNotFound
	}
	if err != nil {
		return ScheduledTask{}, fmt.Errorf("store: get scheduled task %q: %w", id, err)
	}
	return t, nil
}

// ListScheduledTasksForService returns every scheduled task belonging to
// serviceName, oldest first (creation order, the same order
// ListBackupTargets uses for its own listing).
func (db *DB) ListScheduledTasksForService(ctx context.Context, serviceName string) ([]ScheduledTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, name, command, schedule, enabled, timeout_seconds, created_at, updated_at
		FROM scheduled_tasks
		WHERE service_name = ?
		ORDER BY created_at
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled tasks for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		if err := rows.Scan(&t.ID, &t.ServiceName, &t.Name, &t.Command, &t.Schedule, &t.Enabled, &t.TimeoutSeconds, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan scheduled task row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled task rows: %w", err)
	}
	return out, nil
}

// ListEnabledScheduledTasks returns every enabled scheduled task across
// every service, the read internal/scheduledtask.Scheduler's own Tick
// evaluates once per tick, mirroring ListScheduledDatabases' identical
// role for internal/backup.Scheduler.
func (db *DB) ListEnabledScheduledTasks(ctx context.Context) ([]ScheduledTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, name, command, schedule, enabled, timeout_seconds, created_at, updated_at
		FROM scheduled_tasks
		WHERE enabled = 1
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list enabled scheduled tasks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		if err := rows.Scan(&t.ID, &t.ServiceName, &t.Name, &t.Command, &t.Schedule, &t.Enabled, &t.TimeoutSeconds, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan scheduled task row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled task rows: %w", err)
	}
	return out, nil
}

// UpdateScheduledTask replaces name/command/schedule/enabled/timeout_seconds
// for an existing row, identified by t.ID, and stamps updated_at.
// ServiceName/CreatedAt are never touched: an update edits a task's
// definition, not which app it belongs to. Returns
// ErrScheduledTaskNotFound if id doesn't exist.
func (db *DB) UpdateScheduledTask(ctx context.Context, t ScheduledTask) error {
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET name = ?, command = ?, schedule = ?, enabled = ?, timeout_seconds = ?, updated_at = ?
		WHERE id = ?
	`, t.Name, t.Command, t.Schedule, t.Enabled, t.TimeoutSeconds, t.UpdatedAt, t.ID)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", t.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", t.ID, err)
	}
	if n == 0 {
		return ErrScheduledTaskNotFound
	}
	return nil
}

// DeleteScheduledTask removes a scheduled task row and, via
// migrations/0047's ON DELETE CASCADE, every scheduled_task_runs row
// that references it. Returns ErrScheduledTaskNotFound if id doesn't
// exist.
func (db *DB) DeleteScheduledTask(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete scheduled task %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete scheduled task %q: %w", id, err)
	}
	if n == 0 {
		return ErrScheduledTaskNotFound
	}
	return nil
}

// ScheduledTaskRun is one attempted run of one ScheduledTask, the
// scheduled-task counterpart of BackupHistory. Error is empty unless
// Status is ScheduledTaskRunStatusFailed.
type ScheduledTaskRun struct {
	ID              string
	ScheduledTaskID string
	Status          string
	ExitCode        int
	Output          string
	Error           string
	StartedAt       string
	FinishedAt      string
}

// StartScheduledTaskRun records a run beginning, status
// ScheduledTaskRunStatusRunning, before the command has actually
// started, the same "write running eagerly" reasoning
// StartBackupHistory's own doc comment gives.
func (db *DB) StartScheduledTaskRun(ctx context.Context, r ScheduledTaskRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO scheduled_task_runs (id, scheduled_task_id, status, exit_code, output, error, started_at, finished_at)
		VALUES (?, ?, ?, 0, '', '', ?, '')
	`, r.ID, r.ScheduledTaskID, ScheduledTaskRunStatusRunning, r.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start scheduled task run %q: %w", r.ID, err)
	}
	return nil
}

// FinishScheduledTaskRun updates a running row to its final status.
// Returns ErrScheduledTaskRunNotFound if id doesn't exist.
func (db *DB) FinishScheduledTaskRun(ctx context.Context, id, status string, exitCode int, output, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_task_runs
		SET status = ?, exit_code = ?, output = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, exitCode, output, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish scheduled task run %q: %w", id, err)
	}
	if n == 0 {
		return ErrScheduledTaskRunNotFound
	}
	return nil
}

// ListScheduledTaskRuns returns every run of taskID, newest first
// (migrations/0047's own index is built for exactly this query shape).
func (db *DB) ListScheduledTaskRuns(ctx context.Context, taskID string) ([]ScheduledTaskRun, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, scheduled_task_id, status, exit_code, output, error, started_at, finished_at
		FROM scheduled_task_runs
		WHERE scheduled_task_id = ?
		ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled task runs for %q: %w", taskID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ScheduledTaskRun
	for rows.Next() {
		var r ScheduledTaskRun
		if err := rows.Scan(&r.ID, &r.ScheduledTaskID, &r.Status, &r.ExitCode, &r.Output, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan scheduled task run row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled task run rows: %w", err)
	}
	return out, nil
}
