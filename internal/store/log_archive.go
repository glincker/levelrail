package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrLogArchivePolicyNotFound is returned when no policy exists for an app scope.
var ErrLogArchivePolicyNotFound = errors.New("store: log archive policy not found")

// LogArchivePolicy schedules node-local logs to a storage destination.
// AppName is empty for the global policy covering every app.
type LogArchivePolicy struct {
	ID              string
	AppName         string
	TargetID        string
	Enabled         bool
	IntervalSeconds int64
	RetentionDays   int
	WatermarkNs     int64
	LastRunAt       string
	LastSuccessAt   string
	LastError       string
	CreatedAt       string
}

// LogArchiveRun is one archive attempt.
type LogArchiveRun struct {
	ID         string
	PolicyID   string
	AppName    string
	TargetID   string
	Kind       string
	FromNs     int64
	ToNs       int64
	Status     string
	Objects    int
	Lines      int64
	Bytes      int64
	Error      string
	StartedAt  string
	FinishedAt string
}

const logArchivePolicyCols = `id, app_name, target_id, enabled, interval_seconds, retention_days,
	watermark_ns, last_run_at, last_success_at, last_error, created_at`

func scanLogArchivePolicy(sc rowScanner) (LogArchivePolicy, error) {
	var p LogArchivePolicy
	var enabled int
	err := sc.Scan(&p.ID, &p.AppName, &p.TargetID, &enabled, &p.IntervalSeconds, &p.RetentionDays,
		&p.WatermarkNs, &p.LastRunAt, &p.LastSuccessAt, &p.LastError, &p.CreatedAt)
	p.Enabled = enabled != 0
	return p, err
}

// UpsertLogArchivePolicy creates the policy for p.AppName or updates its
// destination, cadence and retention while keeping the watermark.
func (db *DB) UpsertLogArchivePolicy(ctx context.Context, p LogArchivePolicy) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO log_archive_policies (id, app_name, target_id, enabled, interval_seconds, retention_days, watermark_ns, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET target_id = excluded.target_id, enabled = excluded.enabled,
			interval_seconds = excluded.interval_seconds, retention_days = excluded.retention_days
	`, p.ID, p.AppName, p.TargetID, boolInt(p.Enabled), p.IntervalSeconds, p.RetentionDays, p.WatermarkNs, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: upsert log archive policy %q: %w", p.AppName, err)
	}
	return nil
}

// GetLogArchivePolicy returns the policy for an app scope (empty means global).
func (db *DB) GetLogArchivePolicy(ctx context.Context, appName string) (LogArchivePolicy, error) {
	p, err := scanLogArchivePolicy(db.QueryRowContext(ctx,
		`SELECT `+logArchivePolicyCols+` FROM log_archive_policies WHERE app_name = ?`, appName))
	if errors.Is(err, sql.ErrNoRows) {
		return LogArchivePolicy{}, ErrLogArchivePolicyNotFound
	}
	if err != nil {
		return LogArchivePolicy{}, fmt.Errorf("store: get log archive policy %q: %w", appName, err)
	}
	return p, nil
}

// ListLogArchivePolicies returns every policy, global first.
func (db *DB) ListLogArchivePolicies(ctx context.Context) ([]LogArchivePolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+logArchivePolicyCols+` FROM log_archive_policies ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list log archive policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LogArchivePolicy
	for rows.Next() {
		p, err := scanLogArchivePolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan log archive policy: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate log archive policies: %w", err)
	}
	return out, nil
}

// DeleteLogArchivePolicy removes the policy for an app scope.
func (db *DB) DeleteLogArchivePolicy(ctx context.Context, appName string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM log_archive_policies WHERE app_name = ?`, appName)
	if err != nil {
		return fmt.Errorf("store: delete log archive policy %q: %w", appName, err)
	}
	return rowsAffectedOrNotFound(res, ErrLogArchivePolicyNotFound, "delete log archive policy %q", appName)
}

// CountLogArchivePoliciesForTarget reports how many policies reference a target.
func (db *DB) CountLogArchivePoliciesForTarget(ctx context.Context, targetID string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM log_archive_policies WHERE target_id = ?`, targetID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count log archive policies for %q: %w", targetID, err)
	}
	return n, nil
}

// RecordLogArchivePolicyRun stores the outcome of a scheduled run. The
// watermark only moves forward, so a failed window is retried.
func (db *DB) RecordLogArchivePolicyRun(ctx context.Context, id, at string, watermarkNs int64, runErr string) error {
	var err error
	if runErr == "" {
		_, err = db.ExecContext(ctx, `UPDATE log_archive_policies SET last_run_at = ?, last_success_at = ?,
			last_error = '', watermark_ns = MAX(watermark_ns, ?) WHERE id = ?`, at, at, watermarkNs, id)
	} else {
		_, err = db.ExecContext(ctx, `UPDATE log_archive_policies SET last_run_at = ?, last_error = ?,
			watermark_ns = MAX(watermark_ns, ?) WHERE id = ?`, at, runErr, watermarkNs, id)
	}
	if err != nil {
		return fmt.Errorf("store: record log archive policy run %q: %w", id, err)
	}
	return nil
}

// InsertLogArchiveRun starts a run row.
func (db *DB) InsertLogArchiveRun(ctx context.Context, r LogArchiveRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO log_archive_runs (id, policy_id, app_name, target_id, kind, from_ns, to_ns, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.PolicyID, r.AppName, r.TargetID, r.Kind, r.FromNs, r.ToNs, r.Status, r.StartedAt)
	if err != nil {
		return fmt.Errorf("store: insert log archive run %q: %w", r.ID, err)
	}
	return nil
}

// FinishLogArchiveRun closes a run row with its totals.
func (db *DB) FinishLogArchiveRun(ctx context.Context, r LogArchiveRun) error {
	_, err := db.ExecContext(ctx, `UPDATE log_archive_runs SET status = ?, to_ns = ?, objects = ?, lines = ?,
		bytes = ?, error = ?, finished_at = ? WHERE id = ?`,
		r.Status, r.ToNs, r.Objects, r.Lines, r.Bytes, r.Error, r.FinishedAt, r.ID)
	if err != nil {
		return fmt.Errorf("store: finish log archive run %q: %w", r.ID, err)
	}
	return nil
}

// ListLogArchiveRuns returns the newest runs for an app scope; all=true ignores the scope.
func (db *DB) ListLogArchiveRuns(ctx context.Context, appName string, all bool, limit int) ([]LogArchiveRun, error) {
	q := `SELECT id, policy_id, app_name, target_id, kind, from_ns, to_ns, status, objects, lines, bytes, error, started_at, finished_at
		FROM log_archive_runs`
	args := []any{}
	if !all {
		q += ` WHERE app_name = ?`
		args = append(args, appName)
	}
	q += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list log archive runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LogArchiveRun
	for rows.Next() {
		var r LogArchiveRun
		if err := rows.Scan(&r.ID, &r.PolicyID, &r.AppName, &r.TargetID, &r.Kind, &r.FromNs, &r.ToNs, &r.Status,
			&r.Objects, &r.Lines, &r.Bytes, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan log archive run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate log archive runs: %w", err)
	}
	return out, nil
}

// FailStaleLogArchiveRuns marks runs left 'running' by a previous process as failed.
func (db *DB) FailStaleLogArchiveRuns(ctx context.Context, at string) error {
	_, err := db.ExecContext(ctx, `UPDATE log_archive_runs SET status = 'failed', error = 'interrupted by restart',
		finished_at = ? WHERE status = 'running'`, at)
	if err != nil {
		return fmt.Errorf("store: fail stale log archive runs: %w", err)
	}
	return nil
}
