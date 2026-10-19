package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Database password rotation states. Pending means the engine may already
// hold the new password, so the next pass must settle it.
const (
	DBPasswordRotationIdle    = "idle"
	DBPasswordRotationPending = "pending"
	DBPasswordRotationFailed  = "failed"
)

// DatabasePasswordRotation is one database's rotation schedule and progress.
// It never carries a password.
type DatabasePasswordRotation struct {
	Database      string
	IntervalDays  int
	State         string
	LastRotatedAt time.Time
	LastError     string
	PendingSince  time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const dbPasswordRotationColumns = `database_name, interval_days, state, last_rotated_at, last_error, pending_since, created_at, updated_at`

func scanDBPasswordRotation(row interface{ Scan(...any) error }) (DatabasePasswordRotation, error) {
	var r DatabasePasswordRotation
	var last, pending, created, updated string
	if err := row.Scan(&r.Database, &r.IntervalDays, &r.State, &last, &r.LastError, &pending, &created, &updated); err != nil {
		return DatabasePasswordRotation{}, err
	}
	var err error
	if r.LastRotatedAt, err = parseRotationTime(last); err != nil {
		return DatabasePasswordRotation{}, err
	}
	if r.PendingSince, err = parseRotationTime(pending); err != nil {
		return DatabasePasswordRotation{}, err
	}
	if r.CreatedAt, err = parseRotationTime(created); err != nil {
		return DatabasePasswordRotation{}, err
	}
	if r.UpdatedAt, err = parseRotationTime(updated); err != nil {
		return DatabasePasswordRotation{}, err
	}
	return r, nil
}

// GetDatabasePasswordRotation returns the row for a database, or a zero
// idle value (never an error) when none exists yet.
func (db *DB) GetDatabasePasswordRotation(ctx context.Context, name string) (DatabasePasswordRotation, error) {
	row := db.QueryRowContext(ctx, `SELECT `+dbPasswordRotationColumns+` FROM database_password_rotation WHERE database_name = ?`, name)
	r, err := scanDBPasswordRotation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabasePasswordRotation{Database: name, State: DBPasswordRotationIdle}, nil
	}
	if err != nil {
		return DatabasePasswordRotation{}, fmt.Errorf("store: get database password rotation %q: %w", name, err)
	}
	return r, nil
}

// ListDatabasePasswordRotations returns every row, ordered by database.
func (db *DB) ListDatabasePasswordRotations(ctx context.Context) ([]DatabasePasswordRotation, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+dbPasswordRotationColumns+` FROM database_password_rotation ORDER BY database_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list database password rotations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DatabasePasswordRotation
	for rows.Next() {
		r, err := scanDBPasswordRotation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan database password rotation: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetDatabasePasswordRotationInterval sets the scheduled interval in days
// (0 turns the schedule off), creating the row on first use.
func (db *DB) SetDatabasePasswordRotationInterval(ctx context.Context, name string, days int, now time.Time) error {
	ts := formatRotationTime(now)
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_password_rotation (database_name, interval_days, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (database_name) DO UPDATE SET interval_days = excluded.interval_days, updated_at = excluded.updated_at
	`, name, days, ts, ts)
	if err != nil {
		return fmt.Errorf("store: set database password rotation interval %q: %w", name, err)
	}
	return nil
}

// SaveDatabasePasswordRotationState records one state transition. A zero
// lastRotated leaves the stored value alone.
func (db *DB) SaveDatabasePasswordRotationState(ctx context.Context, name, state, reason string, lastRotated, pendingSince, now time.Time) error {
	ts := formatRotationTime(now)
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_password_rotation (database_name, state, last_error, last_rotated_at, pending_since, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (database_name) DO UPDATE SET
			state = excluded.state,
			last_error = excluded.last_error,
			last_rotated_at = CASE WHEN excluded.last_rotated_at = '' THEN database_password_rotation.last_rotated_at ELSE excluded.last_rotated_at END,
			pending_since = excluded.pending_since,
			updated_at = excluded.updated_at
	`, name, state, reason, formatRotationTime(lastRotated), formatRotationTime(pendingSince), ts, ts)
	if err != nil {
		return fmt.Errorf("store: save database password rotation state %q: %w", name, err)
	}
	return nil
}

// DeleteDatabasePasswordRotation drops a database's row, idempotently.
func (db *DB) DeleteDatabasePasswordRotation(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM database_password_rotation WHERE database_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete database password rotation %q: %w", name, err)
	}
	return nil
}
