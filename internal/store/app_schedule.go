package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/idgen"
)

// Statuses recorded on an app_schedule_history row.
const (
	AppScheduleHistoryFired         = "fired"
	AppScheduleHistorySkippedFreeze = "skipped_freeze"
	AppScheduleHistoryFailed        = "failed"
)

const appScheduleHistoryIDPrefix = "ash_"

// ErrAppScheduleNotFound is returned by GetAppSchedule when no schedule is
// configured for a service.
var ErrAppScheduleNotFound = errors.New("store: app schedule not found")

// AppSchedule is one app's configured recurring redeploy
// (migrations/0250_app_schedules.sql): the latest commit on Branch is
// deployed whenever Cron fires, evaluated in Timezone.
type AppSchedule struct {
	ServiceName string
	Cron        string
	Branch      string
	Timezone    string
	Enabled     bool
	// NextFireAt is nil until internal/scheduledeploy.Scheduler has armed
	// this schedule at least once; see that package's own doc comment for
	// why this is persisted rather than kept only in memory.
	NextFireAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AppScheduleHistoryEntry is one recorded evaluation of a due schedule:
// fired, skipped because a freeze window was active, or failed.
type AppScheduleHistoryEntry struct {
	ID           string
	ServiceName  string
	ScheduledFor time.Time
	FiredAt      time.Time
	Status       string
	Reason       string
	CreatedAt    time.Time
}

// NewAppScheduleHistoryID mints a random appScheduleHistoryIDPrefix ID via idgen.
func NewAppScheduleHistoryID() (string, error) {
	id, err := idgen.New(appScheduleHistoryIDPrefix)
	if err != nil {
		return "", fmt.Errorf("store: mint app schedule history id: %w", err)
	}
	return id, nil
}

// GetAppSchedule returns serviceName's configured schedule, or
// ErrAppScheduleNotFound if none is set.
func (db *DB) GetAppSchedule(ctx context.Context, serviceName string) (*AppSchedule, error) {
	row := db.QueryRowContext(ctx, `
		SELECT service_name, cron, branch, timezone, enabled, next_fire_at, created_at, updated_at
		FROM app_schedules WHERE service_name = ?
	`, serviceName)
	s, err := scanAppSchedule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAppScheduleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get app schedule for %q: %w", serviceName, err)
	}
	return s, nil
}

func scanAppSchedule(scan func(dest ...any) error) (*AppSchedule, error) {
	var s AppSchedule
	var next sql.NullString
	var created, updated string
	if err := scan(&s.ServiceName, &s.Cron, &s.Branch, &s.Timezone, &s.Enabled, &next, &created, &updated); err != nil {
		return nil, err
	}
	if next.Valid && next.String != "" {
		t, err := time.Parse(time.RFC3339Nano, next.String)
		if err != nil {
			return nil, fmt.Errorf("parse next_fire_at: %w", err)
		}
		s.NextFireAt = &t
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	s.CreatedAt, s.UpdatedAt = createdAt, updatedAt
	return &s, nil
}

// SetAppSchedule upserts serviceName's schedule. next_fire_at always
// resets to NULL: cron/branch/timezone may have just changed, so any
// previously armed time was computed against config that no longer
// applies. The scheduler re-arms it, unfired, on its next tick, the same
// "a freshly armed schedule never fires on sight" rule it applies to a
// schedule it is seeing for the first time.
func (db *DB) SetAppSchedule(ctx context.Context, serviceName, cronExpr, branch, timezone string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_schedules (service_name, cron, branch, timezone, enabled, next_fire_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET
			cron = excluded.cron,
			branch = excluded.branch,
			timezone = excluded.timezone,
			enabled = excluded.enabled,
			next_fire_at = NULL,
			updated_at = excluded.updated_at
	`, serviceName, cronExpr, branch, timezone, enabled, now, now)
	if err != nil {
		return fmt.Errorf("store: set app schedule for %q: %w", serviceName, err)
	}
	return nil
}

// ListEnabledAppSchedules returns every currently enabled app schedule,
// re-derived fresh on every call: internal/scheduledeploy.Scheduler calls
// this once per tick, the same "no cached membership" shape
// internal/scheduledtask.ScheduleStore's own doc comment establishes.
func (db *DB) ListEnabledAppSchedules(ctx context.Context) ([]AppSchedule, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, cron, branch, timezone, enabled, next_fire_at, created_at, updated_at
		FROM app_schedules WHERE enabled = 1
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list enabled app schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppSchedule
	for rows.Next() {
		s, err := scanAppSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan app schedule: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app schedules: %w", err)
	}
	return out, nil
}

// ArmAppScheduleNextRun records the next time serviceName's schedule
// should fire. A no-op (not an error) once the schedule has been deleted
// or disabled out from under an in-flight tick.
func (db *DB) ArmAppScheduleNextRun(ctx context.Context, serviceName string, next time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE app_schedules SET next_fire_at = ?, updated_at = ? WHERE service_name = ?
	`, next.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), serviceName)
	if err != nil {
		return fmt.Errorf("store: arm app schedule next run for %q: %w", serviceName, err)
	}
	return nil
}

// RecordAppScheduleHistory inserts e, minting its ID and timestamps when unset.
func (db *DB) RecordAppScheduleHistory(ctx context.Context, e AppScheduleHistoryEntry) error {
	if e.ID == "" {
		id, err := NewAppScheduleHistoryID()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_schedule_history (id, service_name, scheduled_for, fired_at, status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.ServiceName, e.ScheduledFor.UTC().Format(time.RFC3339Nano), e.FiredAt.UTC().Format(time.RFC3339Nano),
		e.Status, e.Reason, e.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: record app schedule history for %q: %w", e.ServiceName, err)
	}
	return nil
}

// ListAppScheduleHistory returns serviceName's most recent schedule
// evaluations, newest first, capped at limit (defaulted/clamped by the
// caller).
func (db *DB) ListAppScheduleHistory(ctx context.Context, serviceName string, limit int) ([]AppScheduleHistoryEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, scheduled_for, fired_at, status, reason, created_at
		FROM app_schedule_history WHERE service_name = ?
		ORDER BY fired_at DESC, id DESC LIMIT ?
	`, serviceName, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list app schedule history for %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppScheduleHistoryEntry
	for rows.Next() {
		var e AppScheduleHistoryEntry
		var scheduledFor, firedAt, createdAt string
		if err := rows.Scan(&e.ID, &e.ServiceName, &scheduledFor, &firedAt, &e.Status, &e.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan app schedule history: %w", err)
		}
		if e.ScheduledFor, err = time.Parse(time.RFC3339Nano, scheduledFor); err != nil {
			return nil, fmt.Errorf("parse scheduled_for: %w", err)
		}
		if e.FiredAt, err = time.Parse(time.RFC3339Nano, firedAt); err != nil {
			return nil, fmt.Errorf("parse fired_at: %w", err)
		}
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app schedule history: %w", err)
	}
	return out, nil
}
