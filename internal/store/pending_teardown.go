package store

import (
	"context"
	"fmt"
	"time"
)

// PendingTeardown is a deleted app whose containers are not yet confirmed gone.
type PendingTeardown struct {
	Name          string
	NodeID        string
	CreatedAt     time.Time
	Attempts      int
	LastError     string
	LastAttemptAt *time.Time
}

// AddPendingTeardown records name as awaiting teardown on nodeID. Idempotent.
func (db *DB) AddPendingTeardown(ctx context.Context, name, nodeID string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO pending_teardowns (name, node_id, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET node_id = excluded.node_id`,
		name, nodeID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: add pending teardown %q: %w", name, err)
	}
	return nil
}

// DeletePendingTeardown removes the tombstone for name. Idempotent.
func (db *DB) DeletePendingTeardown(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM pending_teardowns WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: delete pending teardown %q: %w", name, err)
	}
	return nil
}

// RecordPendingTeardownFailure bumps the attempt count and stores cause.
func (db *DB) RecordPendingTeardownFailure(ctx context.Context, name, cause string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE pending_teardowns SET attempts = attempts + 1, last_error = ?, last_attempt_at = ? WHERE name = ?`,
		cause, time.Now().UTC().Format(time.RFC3339Nano), name)
	if err != nil {
		return fmt.Errorf("store: record pending teardown failure %q: %w", name, err)
	}
	return nil
}

// ListPendingTeardowns returns every tombstone, oldest first.
func (db *DB) ListPendingTeardowns(ctx context.Context) ([]PendingTeardown, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, node_id, created_at, attempts, last_error, last_attempt_at FROM pending_teardowns ORDER BY created_at, name`)
	if err != nil {
		return nil, fmt.Errorf("store: list pending teardowns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PendingTeardown
	for rows.Next() {
		var (
			p        PendingTeardown
			created  string
			lastAtNS *string
		)
		if err := rows.Scan(&p.Name, &p.NodeID, &created, &p.Attempts, &p.LastError, &lastAtNS); err != nil {
			return nil, fmt.Errorf("store: scan pending teardown: %w", err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if lastAtNS != nil {
			if t, err := time.Parse(time.RFC3339Nano, *lastAtNS); err == nil {
				p.LastAttemptAt = &t
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate pending teardowns: %w", err)
	}
	return out, nil
}
