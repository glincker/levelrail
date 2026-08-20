package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Scheduled task run statuses (migrations/0048_scheduled_tasks.sql's
// last_run_status column, no CHECK constraint since this is an
// application-level enum, not something a foreign key or another table
// joins against). ScheduledTaskStatusContainerNotRunning is distinct
// from ScheduledTaskStatusFailed so the dashboard can badge "the
// container isn't up" differently from "the command itself failed",
// exactly the "clear, visible signal" this feature's own spec asked
// for rather than folding that case into a generic failure.
const (
	ScheduledTaskStatusSuccess             = "success"
	ScheduledTaskStatusFailed              = "failed"
	ScheduledTaskStatusTimeout             = "timeout"
	ScheduledTaskStatusContainerNotRunning = "container_not_running"
)

// ErrScheduledTaskNotFound is returned by GetScheduledTask,
// UpdateScheduledTask, RecordScheduledTaskRun, and DeleteScheduledTask
// when id doesn't match any row.
var ErrScheduledTaskNotFound = errors.New("store: scheduled task not found")

// ScheduledTask is one cron-scheduled command an operator runs inside a
// service's currently running container. See migrations/
// 0048_scheduled_tasks.sql for the full field-by-field reasoning.
type ScheduledTask struct {
	ID          string
	ServiceName string
	Command     []string
	Schedule    string
	Enabled     bool

	// LastRunAt is nil until this task has run at least once (scheduled
	// or manual "run now"), the same "zero value means never happened
	// yet" convention APIToken's own LastUsedAt already establishes for
	// state that starts out unset.
	LastRunAt     *time.Time
	LastRunStatus string
	// LastRunOutput is already bounded by the runner before it reaches
	// this field (see this table's own migration comment); this struct
	// never re-truncates it.
	LastRunOutput string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// SaveScheduledTask inserts a new scheduled task row. ID is minted by
// the caller (internal/api, the same "generate before the INSERT"
// pattern SaveBackupTarget's own doc comment establishes), and the
// last-run fields start empty: a freshly created task has never run.
func (db *DB) SaveScheduledTask(ctx context.Context, t ScheduledTask) error {
	cmdJSON, err := json.Marshal(t.Command)
	if err != nil {
		return fmt.Errorf("store: save scheduled task %q: encode command: %w", t.ID, err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (id, service_name, command, schedule, enabled, last_run_at, last_run_status, last_run_output, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULL, '', '', ?, ?)
	`, t.ID, t.ServiceName, string(cmdJSON), t.Schedule, t.Enabled, formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save scheduled task %q: %w", t.ID, err)
	}
	return nil
}

// GetScheduledTask returns the scheduled task with this ID, or
// ErrScheduledTaskNotFound.
func (db *DB) GetScheduledTask(ctx context.Context, id string) (ScheduledTask, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, service_name, command, schedule, enabled, last_run_at, last_run_status, last_run_output, created_at, updated_at
		FROM scheduled_tasks
		WHERE id = ?
	`, id)
	t, err := scanScheduledTask(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTask{}, ErrScheduledTaskNotFound
	}
	if err != nil {
		return ScheduledTask{}, fmt.Errorf("store: get scheduled task %q: %w", id, err)
	}
	return *t, nil
}

// ListScheduledTasksForService returns every scheduled task for
// serviceName, oldest first, the same creation-order convention
// ListBackupTargets already uses.
func (db *DB) ListScheduledTasksForService(ctx context.Context, serviceName string) ([]ScheduledTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, command, schedule, enabled, last_run_at, last_run_status, last_run_output, created_at, updated_at
		FROM scheduled_tasks
		WHERE service_name = ?
		ORDER BY created_at
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled tasks for %q: %w", serviceName, err)
	}
	return scanScheduledTasks(rows)
}

// ListEnabledScheduledTasks returns every scheduled task with Enabled
// true, across every service: internal/scheduledtask.Scheduler.Tick's
// own per-tick query (idx_scheduled_tasks_enabled, migrations/
// 0048_scheduled_tasks.sql, supports exactly this filter).
func (db *DB) ListEnabledScheduledTasks(ctx context.Context) ([]ScheduledTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, command, schedule, enabled, last_run_at, last_run_status, last_run_output, created_at, updated_at
		FROM scheduled_tasks
		WHERE enabled = 1
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list enabled scheduled tasks: %w", err)
	}
	return scanScheduledTasks(rows)
}

// UpdateScheduledTask updates a task's own editable fields (Command,
// Schedule, Enabled): it never touches ServiceName (an existing task
// cannot be reassigned to a different app; delete and recreate instead)
// or the last-run fields (RecordScheduledTaskRun's job, not this
// method's). Returns ErrScheduledTaskNotFound if id doesn't exist.
func (db *DB) UpdateScheduledTask(ctx context.Context, id string, command []string, schedule string, enabled bool, updatedAt time.Time) error {
	cmdJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: encode command: %w", id, err)
	}

	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET command = ?, schedule = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, string(cmdJSON), schedule, enabled, formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "update scheduled task %q", id)
}

// RecordScheduledTaskRun writes the outcome of one run (scheduled or
// manual "run now") back onto a task's own row: the same in-place
// "latest attempt" shape this table's own migration comment explains,
// not an append-only history. Returns ErrScheduledTaskNotFound if id
// doesn't exist, e.g. the task was deleted between being picked up by a
// tick and this call.
func (db *DB) RecordScheduledTaskRun(ctx context.Context, id string, ranAt time.Time, status, output string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET last_run_at = ?, last_run_status = ?, last_run_output = ?, updated_at = ?
		WHERE id = ?
	`, formatTime(ranAt), status, output, formatTime(ranAt), id)
	if err != nil {
		return fmt.Errorf("store: record scheduled task run %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "record scheduled task run %q", id)
}

// DeleteScheduledTask removes a scheduled task row. Returns
// ErrScheduledTaskNotFound if id doesn't exist.
func (db *DB) DeleteScheduledTask(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete scheduled task %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "delete scheduled task %q", id)
}

func scanScheduledTasks(rows *sql.Rows) ([]ScheduledTask, error) {
	defer func() {
		_ = rows.Close()
	}()

	var out []ScheduledTask
	for rows.Next() {
		t, err := scanScheduledTask(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan scheduled task row: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled task rows: %w", err)
	}
	return out, nil
}

func scanScheduledTask(scan func(dest ...any) error) (*ScheduledTask, error) {
	var (
		t                    ScheduledTask
		cmdJSON              string
		enabled              int
		lastRunAt            sql.NullString
		createdAt, updatedAt string
	)
	if err := scan(&t.ID, &t.ServiceName, &cmdJSON, &t.Schedule, &enabled, &lastRunAt, &t.LastRunStatus, &t.LastRunOutput, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(cmdJSON), &t.Command); err != nil {
		return nil, fmt.Errorf("unmarshal command: %w", err)
	}
	t.Enabled = enabled != 0

	var err error
	t.LastRunAt, err = parseTimePtr(lastRunAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_run_at: %w", err)
	}

	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &t, nil
}
