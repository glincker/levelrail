package store

import (
	"context"
	"fmt"
	"time"
)

// Build queue states. Paused and draining both hold new builds; draining
// says the operator is waiting for the running ones to finish.
const (
	BuildQueueRunning  = "running"
	BuildQueuePaused   = "paused"
	BuildQueueDraining = "draining"
)

// BuildQueueState is the persisted build queue switch.
type BuildQueueState struct {
	State  string
	Reason string
	Actor  string
	Since  time.Time
}

// BuildQueueEvent is one recorded change of the build queue state.
type BuildQueueEvent struct {
	State  string
	Reason string
	Actor  string
	At     time.Time
}

// GetBuildQueueState returns the saved state; a database without the row reads as running.
func (db *DB) GetBuildQueueState(ctx context.Context) (BuildQueueState, error) {
	var st BuildQueueState
	var since string
	err := db.QueryRowContext(ctx, `SELECT state, reason, actor, since FROM build_queue_state WHERE id = 1`).Scan(&st.State, &st.Reason, &st.Actor, &since)
	if err != nil {
		return BuildQueueState{State: BuildQueueRunning}, fmt.Errorf("store: get build queue state: %w", err)
	}
	if since != "" {
		if t, perr := time.Parse(time.RFC3339Nano, since); perr == nil {
			st.Since = t
		}
	}
	if st.State == "" {
		st.State = BuildQueueRunning
	}
	return st, nil
}

// SetBuildQueueState saves st and appends the change to the event trail in one transaction.
func (db *DB) SetBuildQueueState(ctx context.Context, st BuildQueueState) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set build queue state: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := st.Since.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO build_queue_state (id, state, reason, actor, since) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET state = excluded.state, reason = excluded.reason, actor = excluded.actor, since = excluded.since
	`, st.State, st.Reason, st.Actor, at); err != nil {
		return fmt.Errorf("store: set build queue state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO build_queue_events (state, reason, actor, created_at) VALUES (?, ?, ?, ?)`,
		st.State, st.Reason, st.Actor, at); err != nil {
		return fmt.Errorf("store: record build queue event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set build queue state: commit: %w", err)
	}
	return nil
}

// ListBuildQueueEvents returns the newest changes first, at most limit.
func (db *DB) ListBuildQueueEvents(ctx context.Context, limit int) ([]BuildQueueEvent, error) {
	rows, err := db.QueryContext(ctx, `SELECT state, reason, actor, created_at FROM build_queue_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list build queue events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []BuildQueueEvent{}
	for rows.Next() {
		var e BuildQueueEvent
		var at string
		if err := rows.Scan(&e.State, &e.Reason, &e.Actor, &at); err != nil {
			return nil, fmt.Errorf("store: scan build queue event: %w", err)
		}
		if t, perr := time.Parse(time.RFC3339Nano, at); perr == nil {
			e.At = t
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate build queue events: %w", err)
	}
	return out, nil
}
