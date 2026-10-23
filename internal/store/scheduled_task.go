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
	// ScheduledTaskStatusSkippedConcurrency is recorded when
	// ConcurrencyPolicy is "forbid" and a previous run of the same task
	// was still in flight, so a skipped tick stays visible in run
	// history instead of silently vanishing.
	ScheduledTaskStatusSkippedConcurrency = "skipped_concurrency"
	// ScheduledTaskStatusReplaced is recorded on the run that
	// ConcurrencyPolicy "replace" cancelled to make room for a newer
	// invocation of the same task.
	ScheduledTaskStatusReplaced = "replaced"
	// ScheduledTaskStatusRunning is the in-flight state of a history row.
	ScheduledTaskStatusRunning = "running"
	// ScheduledTaskStatusMissed marks an occurrence the control plane did
	// not run (it was down, and catch-up did not cover it).
	ScheduledTaskStatusMissed = "missed"
	// ScheduledTaskStatusInterrupted marks a run whose control plane
	// restarted mid-flight; it is never re-run, to rule out a double run.
	ScheduledTaskStatusInterrupted = "interrupted"
)

// Scheduled task execution modes and catch-up policies.
const (
	ScheduledTaskModeExec        = "exec"
	ScheduledTaskModeFresh       = "fresh"
	ScheduledTaskCatchUpSkip     = "skip"
	ScheduledTaskCatchUpRunOnce  = "run_once"
	ScheduledTaskSourceAPI       = "api"
	ScheduledTaskSourceSpec      = "spec"
	ScheduledTaskTriggerSchedule = "schedule"
	ScheduledTaskTriggerManual   = "manual"
	ScheduledTaskTriggerAPI      = "api"
)

// Scheduled task concurrency policies (migrations/
// 0101_scheduled_task_concurrency_policy.sql's concurrency_policy
// column): what internal/scheduledtask.Runner does when a task's next
// due run finds a previous invocation of itself still executing.
const (
	ScheduledTaskConcurrencyAllow   = "allow"
	ScheduledTaskConcurrencyForbid  = "forbid"
	ScheduledTaskConcurrencyReplace = "replace"
	// ScheduledTaskConcurrencyQueue waits for the in-flight run to finish
	// and then runs, so overlapping occurrences execute one after another.
	ScheduledTaskConcurrencyQueue = "queue"
)

const scheduledTaskColumns = `id, service_name, command, schedule, enabled, concurrency_policy, last_run_at, last_run_status, last_run_output, consecutive_failures, created_at, updated_at,
	name, mode, timezone, timeout_seconds, max_retries, retry_backoff_seconds, catch_up, catch_up_lookback_seconds,
	cpu_limit_millis, memory_limit_mb, warn_after_seconds, source, armed_at, last_scheduled_for, running_since, consecutive_missed`

func marshalCommand(cmd []string) (string, error) {
	b, err := json.Marshal(cmd)
	if err != nil {
		return "", fmt.Errorf("encode command: %w", err)
	}
	return string(b), nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

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
	// ConcurrencyPolicy governs what happens when this task's next due
	// run finds a previous invocation still executing (internal/
	// scheduledtask.Runner): allow, forbid, or replace. Always one of
	// those three once loaded from the store; SaveScheduledTask and
	// UpdateScheduledTask both default an empty value to allow.
	ConcurrencyPolicy string

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
	// ConsecutiveFailures counts runs in a row that were not
	// ScheduledTaskStatusSuccess, reset to 0 by RecordScheduledTaskRun on
	// a success. What a kind=scheduled_task_failure alert rule
	// (internal/alerting) polls, so repeated failure is visible without
	// this table keeping a full run history.
	ConsecutiveFailures int

	// Name is the job's operator-facing identifier, unique per app when
	// set (what app.yaml's jobs: entries are keyed by).
	Name string
	// Mode is ScheduledTaskModeExec or ScheduledTaskModeFresh.
	Mode string
	// Timezone is an IANA zone the cron expression is evaluated in.
	Timezone string
	// TimeoutSeconds, MaxRetries and RetryBackoffSeconds use 0 as "the
	// control plane default" (env-configured), never as a literal zero.
	TimeoutSeconds      int
	MaxRetries          int
	RetryBackoffSeconds int
	// CatchUp is ScheduledTaskCatchUpSkip or ScheduledTaskCatchUpRunOnce,
	// what to do when the control plane was down across occurrences.
	CatchUp                string
	CatchUpLookbackSeconds int
	// CPULimitMillis and MemoryLimitMB bound a fresh-mode container, 0
	// meaning the app's own limits.
	CPULimitMillis int
	MemoryLimitMB  int
	// WarnAfterSeconds raises the running-too-long alert once a run has
	// been in flight that long, 0 meaning never.
	WarnAfterSeconds int
	// Source is ScheduledTaskSourceAPI or ScheduledTaskSourceSpec: a
	// spec-sourced job is owned by the app.yaml that declared it.
	Source string

	// LastScheduledFor is the cron occurrence the scheduler last handled
	// (ran, skipped, or recorded as missed): the level-triggered
	// high-water mark that makes a restart neither double-run nor
	// silently skip. ArmedAt is when the schedule last became live.
	LastScheduledFor *time.Time
	ArmedAt          *time.Time
	// RunningSince is non-nil while a run is in flight, what the
	// running-too-long alert polls. ConsecutiveMissed counts occurrences
	// the control plane failed to run in a row.
	RunningSince      *time.Time
	ConsecutiveMissed int

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
		INSERT INTO scheduled_tasks (id, service_name, command, schedule, enabled, concurrency_policy, last_run_at, last_run_status, last_run_output, created_at, updated_at,
			name, mode, timezone, timeout_seconds, max_retries, retry_backoff_seconds, catch_up, catch_up_lookback_seconds, cpu_limit_millis, memory_limit_mb, warn_after_seconds, source, armed_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, '', '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.ServiceName, string(cmdJSON), t.Schedule, t.Enabled, normalizeConcurrencyPolicy(t.ConcurrencyPolicy), formatTime(t.CreatedAt), formatTime(t.UpdatedAt),
		t.Name, orDefault(t.Mode, ScheduledTaskModeExec), orDefault(t.Timezone, "UTC"), t.TimeoutSeconds, t.MaxRetries, t.RetryBackoffSeconds,
		orDefault(t.CatchUp, ScheduledTaskCatchUpSkip), t.CatchUpLookbackSeconds, t.CPULimitMillis, t.MemoryLimitMB, t.WarnAfterSeconds,
		orDefault(t.Source, ScheduledTaskSourceAPI), formatTime(t.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: save scheduled task %q: %w", t.ID, err)
	}
	return nil
}

// normalizeConcurrencyPolicy defaults an empty ConcurrencyPolicy to
// ScheduledTaskConcurrencyAllow, the same "empty means today's existing
// behavior" convention this field was added under.
func normalizeConcurrencyPolicy(policy string) string {
	if policy == "" {
		return ScheduledTaskConcurrencyAllow
	}
	return policy
}

// GetScheduledTask returns the scheduled task with this ID, or
// ErrScheduledTaskNotFound.
func (db *DB) GetScheduledTask(ctx context.Context, id string) (ScheduledTask, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+scheduledTaskColumns+`
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
		SELECT `+scheduledTaskColumns+`
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
		SELECT `+scheduledTaskColumns+`
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
// Schedule, Enabled, ConcurrencyPolicy): it never touches ServiceName (an
// existing task cannot be reassigned to a different app; delete and
// recreate instead) or the last-run fields (RecordScheduledTaskRun's job,
// not this method's). An empty concurrencyPolicy defaults to
// ScheduledTaskConcurrencyAllow, the same as SaveScheduledTask. Returns
// ErrScheduledTaskNotFound if id doesn't exist.
func (db *DB) UpdateScheduledTask(ctx context.Context, id string, command []string, schedule string, enabled bool, concurrencyPolicy string, updatedAt time.Time) error {
	cmdJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: encode command: %w", id, err)
	}

	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET command = ?, schedule = ?, enabled = ?, concurrency_policy = ?, updated_at = ?
		WHERE id = ?
	`, string(cmdJSON), schedule, enabled, normalizeConcurrencyPolicy(concurrencyPolicy), formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update scheduled task %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrScheduledTaskNotFound, "update scheduled task %q", id)
}

// RecordScheduledTaskRun writes the outcome of one run (scheduled or
// manual "run now") back onto a task's own row: the same in-place
// "latest attempt" shape this table's own migration comment explains,
// not an append-only history. consecutive_failures resets to 0 on a
// success, stays unchanged on a skipped_concurrency or replaced outcome
// (neither one is a failure of the command itself, just a concurrency-
// policy side effect), and increments otherwise, computed in SQL rather
// than read-then-write so a concurrent run can't race it. Returns
// ErrScheduledTaskNotFound if id doesn't exist, e.g. the task was
// deleted between being picked up by a tick and this call.
func (db *DB) RecordScheduledTaskRun(ctx context.Context, id string, ranAt time.Time, status, output string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET last_run_at = ?, last_run_status = ?, last_run_output = ?, updated_at = ?,
			consecutive_failures = CASE
				WHEN ? = ? THEN 0
				WHEN ? IN (?, ?) THEN consecutive_failures
				ELSE consecutive_failures + 1
			END
		WHERE id = ?
	`, formatTime(ranAt), status, output, formatTime(ranAt),
		status, ScheduledTaskStatusSuccess,
		status, ScheduledTaskStatusSkippedConcurrency, ScheduledTaskStatusReplaced,
		id)
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
	var armedAt, lastScheduled, runningSince sql.NullString
	if err := scan(&t.ID, &t.ServiceName, &cmdJSON, &t.Schedule, &enabled, &t.ConcurrencyPolicy, &lastRunAt, &t.LastRunStatus, &t.LastRunOutput, &t.ConsecutiveFailures, &createdAt, &updatedAt,
		&t.Name, &t.Mode, &t.Timezone, &t.TimeoutSeconds, &t.MaxRetries, &t.RetryBackoffSeconds, &t.CatchUp, &t.CatchUpLookbackSeconds,
		&t.CPULimitMillis, &t.MemoryLimitMB, &t.WarnAfterSeconds, &t.Source, &armedAt, &lastScheduled, &runningSince, &t.ConsecutiveMissed); err != nil {
		return nil, err
	}
	var perr error
	if t.ArmedAt, perr = parseTimePtr(armedAt); perr != nil {
		return nil, fmt.Errorf("parse armed_at: %w", perr)
	}
	if t.LastScheduledFor, perr = parseTimePtr(lastScheduled); perr != nil {
		return nil, fmt.Errorf("parse last_scheduled_for: %w", perr)
	}
	if t.RunningSince, perr = parseTimePtr(runningSince); perr != nil {
		return nil, fmt.Errorf("parse running_since: %w", perr)
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
