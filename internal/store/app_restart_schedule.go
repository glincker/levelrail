package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AppRestartSchedule is a periodic restart of one app (migrations/3234).
// Times are RFC 3339 UTC strings so next_run_at sorts and compares as text.
type AppRestartSchedule struct {
	AppName    string `json:"app_name"`
	Schedule   string `json:"schedule"`
	Timezone   string `json:"timezone"`
	SetBy      string `json:"set_by,omitempty"`
	SetAt      string `json:"set_at"`
	NextRunAt  string `json:"next_run_at"`
	LastRunAt  string `json:"last_run_at,omitempty"`
	LastStatus string `json:"last_status,omitempty"`
	LastReason string `json:"last_reason,omitempty"`
	Failures   int    `json:"failures"`
	// Incarnation is the service life the schedule was set on; another life never inherits it.
	Incarnation string `json:"-"`
}

// Outcomes of one scheduled restart run.
const (
	AppRestartStatusStarted   = "started"
	AppRestartStatusSucceeded = "succeeded"
	AppRestartStatusSkipped   = "skipped"
	AppRestartStatusFailed    = "failed"
)

const appRestartScheduleColumns = `app_name, schedule, timezone, incarnation, set_by, set_at, next_run_at, last_run_at, last_status, last_reason, failures`

func scanAppRestartSchedule(scan func(...any) error) (AppRestartSchedule, error) {
	var s AppRestartSchedule
	err := scan(&s.AppName, &s.Schedule, &s.Timezone, &s.Incarnation, &s.SetBy, &s.SetAt, &s.NextRunAt, &s.LastRunAt, &s.LastStatus, &s.LastReason, &s.Failures)
	return s, err
}

// GetAppRestartSchedule returns the app's restart schedule, or nil when none is set.
func (db *DB) GetAppRestartSchedule(ctx context.Context, app string) (*AppRestartSchedule, error) {
	s, err := scanAppRestartSchedule(db.QueryRowContext(ctx, `SELECT `+appRestartScheduleColumns+` FROM app_restart_schedule WHERE app_name = ?`, app).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get app restart schedule: %w", err)
	}
	return &s, nil
}

// SetAppRestartSchedule saves s and resets its run history and failure count.
func (db *DB) SetAppRestartSchedule(ctx context.Context, s AppRestartSchedule) error {
	_, err := db.ExecContext(ctx, `INSERT INTO app_restart_schedule (`+appRestartScheduleColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', 0)
		ON CONFLICT(app_name) DO UPDATE SET schedule = excluded.schedule, timezone = excluded.timezone,
		incarnation = excluded.incarnation, set_by = excluded.set_by, set_at = excluded.set_at,
		next_run_at = excluded.next_run_at, last_run_at = '', last_status = '', last_reason = '', failures = 0`,
		s.AppName, s.Schedule, s.Timezone, s.Incarnation, s.SetBy, s.SetAt, s.NextRunAt)
	if err != nil {
		return fmt.Errorf("store: set app restart schedule: %w", err)
	}
	return nil
}

// ClearAppRestartSchedule removes the schedule and reports whether one existed.
func (db *DB) ClearAppRestartSchedule(ctx context.Context, app string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_restart_schedule WHERE app_name = ?`, app)
	if err != nil {
		return false, fmt.Errorf("store: clear app restart schedule: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListDueAppRestartSchedules returns schedules due at or before now, soonest first.
func (db *DB) ListDueAppRestartSchedules(ctx context.Context, now string, limit int) ([]AppRestartSchedule, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+appRestartScheduleColumns+` FROM app_restart_schedule WHERE next_run_at <= ? ORDER BY next_run_at ASC, app_name ASC LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list due app restart schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppRestartSchedule
	for rows.Next() {
		s, err := scanAppRestartSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan app restart schedule: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RecordAppRestartRun stores a run outcome and the next run only when the row
// still holds prev's next_run_at and incarnation, so two sweeps never both fire it.
func (db *DB) RecordAppRestartRun(ctx context.Context, prev AppRestartSchedule, ranAt, status, reason string, failures int, next string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE app_restart_schedule SET last_run_at = ?, last_status = ?, last_reason = ?, failures = ?, next_run_at = ?
		WHERE app_name = ? AND incarnation = ? AND next_run_at = ?`,
		ranAt, status, reason, failures, next, prev.AppName, prev.Incarnation, prev.NextRunAt)
	if err != nil {
		return false, fmt.Errorf("store: record app restart run: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RemoveAppRestartScheduleRow deletes s only while it still names the same life of the app.
func (db *DB) RemoveAppRestartScheduleRow(ctx context.Context, s AppRestartSchedule) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_restart_schedule WHERE app_name = ? AND incarnation = ?`, s.AppName, s.Incarnation); err != nil {
		return fmt.Errorf("store: remove app restart schedule: %w", err)
	}
	return nil
}
