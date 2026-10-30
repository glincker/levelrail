package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

const heartbeatColumns = `SELECT monitor_id, token_hash, armed_at, last_ok_at, last_start_at, last_fail_at FROM monitor_heartbeats`

func scanHeartbeat(row *sql.Row) (monitor.HeartbeatState, error) {
	var h monitor.HeartbeatState
	err := row.Scan(&h.MonitorID, &h.TokenHash, &h.ArmedAt, &h.LastOK, &h.LastStart, &h.LastFail)
	if errors.Is(err, sql.ErrNoRows) {
		return monitor.HeartbeatState{}, monitor.ErrNotFound
	}
	if err != nil {
		return monitor.HeartbeatState{}, fmt.Errorf("store: read heartbeat: %w", err)
	}
	return h, nil
}

// SaveHeartbeatToken stores a new token hash for a monitor, replacing any earlier one and
// resetting the ping history so a rotated job starts from a clean slate.
func (db *DB) SaveHeartbeatToken(ctx context.Context, monitorID, tokenHash string, armedAt int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO monitor_heartbeats (monitor_id, token_hash, armed_at) VALUES (?, ?, ?)
		ON CONFLICT (monitor_id) DO UPDATE SET token_hash = excluded.token_hash, armed_at = excluded.armed_at`,
		monitorID, tokenHash, armedAt)
	if err != nil {
		return fmt.Errorf("store: save heartbeat token %q: %w", monitorID, err)
	}
	return nil
}

// FindHeartbeat looks a heartbeat up by token hash, or returns monitor.ErrNotFound.
func (db *DB) FindHeartbeat(ctx context.Context, tokenHash string) (monitor.HeartbeatState, error) {
	return scanHeartbeat(db.QueryRowContext(ctx, heartbeatColumns+` WHERE token_hash = ?`, tokenHash))
}

// GetHeartbeat reads a monitor's ping history, or returns monitor.ErrNotFound.
func (db *DB) GetHeartbeat(ctx context.Context, monitorID string) (monitor.HeartbeatState, error) {
	return scanHeartbeat(db.QueryRowContext(ctx, heartbeatColumns+` WHERE monitor_id = ?`, monitorID))
}

// RecordHeartbeat stamps a ping signal: start, fail, or a plain success.
func (db *DB) RecordHeartbeat(ctx context.Context, monitorID, signal string, at int64) error {
	col := "last_ok_at"
	switch signal {
	case monitor.PingStart:
		col = "last_start_at"
	case monitor.PingFail:
		col = "last_fail_at"
	}
	if _, err := db.ExecContext(ctx, `UPDATE monitor_heartbeats SET `+col+` = ? WHERE monitor_id = ?`, at, monitorID); err != nil {
		return fmt.Errorf("store: record heartbeat %q: %w", monitorID, err)
	}
	return nil
}
