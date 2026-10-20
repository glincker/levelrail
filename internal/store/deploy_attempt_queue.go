package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DeployReasonCanceled is recorded on canceled attempts.
const DeployReasonCanceled = "Canceled"

func (db *DB) requireDeployAttempt(ctx context.Context, id string) error {
	var one int
	if err := db.QueryRowContext(ctx, `SELECT 1 FROM deploy_attempts WHERE id = ?`, id).Scan(&one); err != nil {
		return ErrDeployAttemptNotFound
	}
	return nil
}

func (db *DB) queryDeployAttempts(ctx context.Context, what, where string, args ...any) ([]DeployAttempt, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+deployAttemptColumns+` FROM deploy_attempts WHERE `+where, args...) //nolint:gosec // where is a package-internal constant clause
	if err != nil {
		return nil, fmt.Errorf("store: list %s deploy attempts: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployAttempt
	for rows.Next() {
		a, err := scanDeployAttempt(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan %s deploy attempt: %w", what, err)
		}
		out = append(out, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate %s deploy attempts: %w", what, err)
	}
	return out, nil
}

// ListQueuedDeployAttempts returns every queued attempt, oldest first.
func (db *DB) ListQueuedDeployAttempts(ctx context.Context) ([]DeployAttempt, error) {
	return db.queryDeployAttempts(ctx, "queued", `status = ? ORDER BY queued_at ASC, started_at ASC, id ASC`, DeployAttemptStatusQueued)
}

// ListRunningDeployAttempts returns every running attempt.
func (db *DB) ListRunningDeployAttempts(ctx context.Context) ([]DeployAttempt, error) {
	return db.queryDeployAttempts(ctx, "running", `status = ? ORDER BY started_at ASC`, DeployAttemptStatusRunning)
}

// StartQueuedDeployAttempt moves a queued attempt to running and stamps its
// real start time. Reports whether it moved, so two drainers never both start it.
func (db *DB) StartQueuedDeployAttempt(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, started_at = ?, reason = ''
		WHERE id = ? AND status = ?
	`, DeployAttemptStatusRunning, now.UTC().Format(time.RFC3339Nano), id, DeployAttemptStatusQueued)
	return movedRows(res, err, "start queued deploy attempt", id)
}

// FinishRunningDeployAttempt finishes an attempt only while it is still
// running, so a queue replay never overwrites an outcome the deploy itself
// already recorded. Reports whether it moved.
func (db *DB) FinishRunningDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, finished_at = ?, error = NULLIF(?, '')
		WHERE id = ? AND status = ?
	`, status, finishedAt.UTC().Format(time.RFC3339Nano), errMsg, id, DeployAttemptStatusRunning)
	return movedRows(res, err, "finish running deploy attempt", id)
}

// CancelDeployAttempt moves an attempt from status from to canceled,
// recording who canceled it. Reports whether it moved.
func (db *DB) CancelDeployAttempt(ctx context.Context, id, from, by string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, reason = ?, canceled_by = ?, finished_at = ?
		WHERE id = ? AND status = ?
	`, DeployAttemptStatusCanceled, DeployReasonCanceled, by, now.UTC().Format(time.RFC3339Nano), id, from)
	return movedRows(res, err, "cancel deploy attempt", id)
}

// SupersedeQueuedDeployAttempt marks a still-queued attempt as replaced by a
// newer one. Reports whether it moved; a started attempt is never touched.
func (db *DB) SupersedeQueuedDeployAttempt(ctx context.Context, id, by string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, reason = ?, superseded_by = ?, finished_at = ?
		WHERE id = ? AND status = ?
	`, DeployAttemptStatusSuperseded, DeployReasonSuperseded, by, now.UTC().Format(time.RFC3339Nano), id, DeployAttemptStatusQueued)
	return movedRows(res, err, "supersede queued deploy attempt", id)
}

func movedRows(res sql.Result, err error, what, id string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("store: %s %q: %w", what, id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: %s %q: rows affected: %w", what, id, err)
	}
	return n > 0, nil
}

// GetServiceCancelSuperseded reports whether name supersedes older queued deploys.
func (db *DB) GetServiceCancelSuperseded(ctx context.Context, name string) (bool, error) {
	var on bool
	err := db.QueryRowContext(ctx, `SELECT cancel_superseded FROM desired_services WHERE name = ?`, name).Scan(&on)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrServiceNotFound
	}
	if err != nil {
		return false, fmt.Errorf("store: get cancel superseded for service %q: %w", name, err)
	}
	return on, nil
}

// SetServiceCancelSuperseded is the only way cancel_superseded changes,
// following the single-purpose setter convention of the other per-app flags.
func (db *DB) SetServiceCancelSuperseded(ctx context.Context, name string, enabled bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET cancel_superseded = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
	moved, err := movedRows(res, err, "update cancel superseded for service", name)
	if err != nil {
		return err
	}
	if !moved {
		return ErrServiceNotFound
	}
	return nil
}
