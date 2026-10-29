package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrRehydrateRunNotFound is returned when no rehydrate run was ever saved.
var ErrRehydrateRunNotFound = errors.New("store: rehydrate run not found")

// RehydrateRun is one saved whole-server recovery report; Report is its JSON.
type RehydrateRun struct {
	ID         string
	State      string
	StartedAt  string
	FinishedAt string
	Report     string
}

// SaveRehydrateRun inserts or replaces a run by ID.
func (db *DB) SaveRehydrateRun(ctx context.Context, r RehydrateRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO rehydrate_runs (id, state, started_at, finished_at, report)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET state = excluded.state, finished_at = excluded.finished_at, report = excluded.report
	`, r.ID, r.State, r.StartedAt, r.FinishedAt, r.Report)
	if err != nil {
		return fmt.Errorf("store: save rehydrate run %q: %w", r.ID, err)
	}
	return nil
}

// LatestRehydrateRun returns the most recently started run.
func (db *DB) LatestRehydrateRun(ctx context.Context) (RehydrateRun, error) {
	var r RehydrateRun
	err := db.QueryRowContext(ctx, `
		SELECT id, state, started_at, finished_at, report FROM rehydrate_runs ORDER BY started_at DESC, id DESC LIMIT 1
	`).Scan(&r.ID, &r.State, &r.StartedAt, &r.FinishedAt, &r.Report)
	if errors.Is(err, sql.ErrNoRows) {
		return RehydrateRun{}, ErrRehydrateRunNotFound
	}
	if err != nil {
		return RehydrateRun{}, fmt.Errorf("store: latest rehydrate run: %w", err)
	}
	return r, nil
}
