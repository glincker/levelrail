package store

import (
	"context"
	"fmt"
	"time"
)

// HeartbeatState is the single dead-man heartbeat row: the body toggle and the last ping outcome.
type HeartbeatState struct {
	BodyEnabled bool
	LastPingAt  time.Time
	LastResult  string
	LastError   string
}

// GetHeartbeatState returns the heartbeat row, seeded by its migration.
func (db *DB) GetHeartbeatState(ctx context.Context) (HeartbeatState, error) {
	var (
		s    HeartbeatState
		body int
		at   string
	)
	err := db.QueryRowContext(ctx, `SELECT body_enabled, last_ping_at, last_result, last_error FROM heartbeat_state WHERE id = 1`).
		Scan(&body, &at, &s.LastResult, &s.LastError)
	if err != nil {
		return HeartbeatState{}, fmt.Errorf("store: get heartbeat state: %w", err)
	}
	s.BodyEnabled = body != 0
	if at != "" {
		s.LastPingAt, _ = time.Parse(time.RFC3339, at)
	}
	return s, nil
}

// SaveHeartbeatState replaces the heartbeat row.
func (db *DB) SaveHeartbeatState(ctx context.Context, s HeartbeatState) error {
	at := ""
	if !s.LastPingAt.IsZero() {
		at = s.LastPingAt.UTC().Format(time.RFC3339)
	}
	body := 0
	if s.BodyEnabled {
		body = 1
	}
	_, err := db.ExecContext(ctx, `UPDATE heartbeat_state SET body_enabled = ?, last_ping_at = ?, last_result = ?, last_error = ? WHERE id = 1`,
		body, at, s.LastResult, s.LastError)
	if err != nil {
		return fmt.Errorf("store: save heartbeat state: %w", err)
	}
	return nil
}
