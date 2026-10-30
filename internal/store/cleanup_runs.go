package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CleanupRun is one cleanup run as recorded in history.
type CleanupRun struct {
	ID         int64
	Target     string
	StartedAt  time.Time
	FinishedAt time.Time
	Days       int
	DryRun     bool
	Rows       int64
	Bytes      int64
	Trigger    string
	Actor      string
	Error      string
}

// CleanupRunFilter narrows ListCleanupRuns. Zero values match everything.
type CleanupRunFilter struct {
	Target string
	Since  time.Time
	Until  time.Time
	DryRun *bool
	// Status is "ok" or "failed"; empty matches both.
	Status string
	Limit  int
	// BeforeID returns runs with a smaller id, for paging newest first.
	BeforeID int64
}

const cleanupRunCols = `id, target, started_at, finished_at, days, dry_run, rows_removed, bytes, trigger_kind, actor, error`

// InsertCleanupRun stores r and returns its id.
func (db *DB) InsertCleanupRun(ctx context.Context, r CleanupRun) (int64, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO cleanup_runs (target, started_at, finished_at, days, dry_run, rows_removed, bytes, trigger_kind, actor, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Target, r.StartedAt.UTC().Format(time.RFC3339Nano), r.FinishedAt.UTC().Format(time.RFC3339Nano), r.Days, boolInt(r.DryRun),
		r.Rows, r.Bytes, r.Trigger, r.Actor, r.Error)
	if err != nil {
		return 0, fmt.Errorf("store: insert cleanup run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: cleanup run id: %w", err)
	}
	return id, nil
}

// ListCleanupRuns returns runs newest first.
func (db *DB) ListCleanupRuns(ctx context.Context, f CleanupRunFilter) ([]CleanupRun, error) {
	var where []string
	var args []any
	if f.Target != "" {
		where = append(where, "target = ?")
		args = append(args, f.Target)
	}
	if !f.Since.IsZero() {
		where = append(where, "started_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		where = append(where, "started_at < ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.DryRun != nil {
		where = append(where, "dry_run = ?")
		args = append(args, boolInt(*f.DryRun))
	}
	switch f.Status {
	case "ok":
		where = append(where, "error = ''")
	case "failed":
		where = append(where, "error != ''")
	}
	if f.BeforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, f.BeforeID)
	}
	q := `SELECT ` + cleanupRunCols + ` FROM cleanup_runs`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY id DESC LIMIT ?`
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)
	return db.queryCleanupRuns(ctx, q, args...)
}

// LatestCleanupRuns returns the newest run of every target that has one.
func (db *DB) LatestCleanupRuns(ctx context.Context) ([]CleanupRun, error) {
	return db.queryCleanupRuns(ctx, `SELECT `+cleanupRunCols+` FROM cleanup_runs WHERE id IN (SELECT MAX(id) FROM cleanup_runs GROUP BY target)`)
}

// CleanupFailedSince reports whether target recorded a failed run at or after since.
func (db *DB) CleanupFailedSince(ctx context.Context, target string, since time.Time) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cleanup_runs WHERE target = ? AND error != '' AND started_at >= ?`,
		target, since.UTC().Format(time.RFC3339Nano)).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: cleanup failures: %w", err)
	}
	return n > 0, nil
}

// PruneCleanupRuns deletes history older than before and returns how many rows went.
func (db *DB) PruneCleanupRuns(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM cleanup_runs WHERE started_at < ?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: prune cleanup runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (db *DB) queryCleanupRuns(ctx context.Context, q string, args ...any) ([]CleanupRun, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list cleanup runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CleanupRun{}
	for rows.Next() {
		var r CleanupRun
		var started, finished string
		var dry int
		if err := rows.Scan(&r.ID, &r.Target, &started, &finished, &r.Days, &dry, &r.Rows, &r.Bytes, &r.Trigger, &r.Actor, &r.Error); err != nil {
			return nil, fmt.Errorf("store: scan cleanup run: %w", err)
		}
		r.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		r.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
		r.DryRun = dry != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list cleanup runs: %w", err)
	}
	return out, nil
}
