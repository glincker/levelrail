package store

import (
	"context"
	"fmt"
	"time"
)

// DatabaseTeardownJob is a container teardown still owed for a database that left desired state.
type DatabaseTeardownJob struct {
	Name      string
	NodeID    string
	Purge     bool
	Attempts  int
	LastError string
	CreatedAt string
}

// SaveDatabaseTeardownJob records a teardown before the desired row goes away; repeats are ignored.
func (db *DB) SaveDatabaseTeardownJob(ctx context.Context, name, nodeID string, purge bool) error {
	p := 0
	if purge {
		p = 1
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO database_teardown_jobs (name, node_id, purge, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name, node_id) DO UPDATE SET purge = MAX(purge, excluded.purge)`,
		name, nodeID, p, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save database teardown job %q: %w", name, err)
	}
	return nil
}

// ListDatabaseTeardownJobs returns pending teardowns, oldest first.
func (db *DB) ListDatabaseTeardownJobs(ctx context.Context) ([]DatabaseTeardownJob, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, node_id, purge, attempts, last_error, created_at FROM database_teardown_jobs ORDER BY created_at, name`)
	if err != nil {
		return nil, fmt.Errorf("store: list database teardown jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DatabaseTeardownJob
	for rows.Next() {
		var j DatabaseTeardownJob
		var purge int
		if err := rows.Scan(&j.Name, &j.NodeID, &purge, &j.Attempts, &j.LastError, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan database teardown job: %w", err)
		}
		j.Purge = purge != 0
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list database teardown jobs: %w", err)
	}
	return out, nil
}

// FinishDatabaseTeardownJob removes a job whose teardown succeeded.
func (db *DB) FinishDatabaseTeardownJob(ctx context.Context, name, nodeID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM database_teardown_jobs WHERE name = ? AND node_id = ?`, name, nodeID); err != nil {
		return fmt.Errorf("store: finish database teardown job %q: %w", name, err)
	}
	return nil
}

// FailDatabaseTeardownJob records a failed attempt so it stays pending.
func (db *DB) FailDatabaseTeardownJob(ctx context.Context, name, nodeID, reason string) error {
	if _, err := db.ExecContext(ctx,
		`UPDATE database_teardown_jobs SET attempts = attempts + 1, last_error = ? WHERE name = ? AND node_id = ?`,
		reason, name, nodeID); err != nil {
		return fmt.Errorf("store: fail database teardown job %q: %w", name, err)
	}
	return nil
}
