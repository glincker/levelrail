package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

const monitorColumns = `SELECT id, name, kind, interval_s, timeout_ms, enabled, spec, created_at FROM monitors`

func scanMonitor(scan func(...any) error) (monitor.Monitor, error) {
	var (
		m                   monitor.Monitor
		interval, timeoutMS int64
		enabled, createdAt  int64
		spec                string
	)
	if err := scan(&m.ID, &m.Name, &m.Kind, &interval, &timeoutMS, &enabled, &spec, &createdAt); err != nil {
		return monitor.Monitor{}, err
	}
	if err := json.Unmarshal([]byte(spec), &m.Spec); err != nil {
		return monitor.Monitor{}, fmt.Errorf("decode spec: %w", err)
	}
	m.Interval = time.Duration(interval) * time.Second
	m.Timeout = time.Duration(timeoutMS) * time.Millisecond
	m.Enabled = enabled != 0
	m.CreatedAt = time.Unix(createdAt, 0).UTC()
	return m, nil
}

// SaveMonitor creates or replaces a monitor definition.
func (db *DB) SaveMonitor(ctx context.Context, m monitor.Monitor) error {
	spec, err := json.Marshal(m.Spec)
	if err != nil {
		return fmt.Errorf("store: encode monitor spec: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO monitors (id, name, kind, interval_s, timeout_ms, enabled, spec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, interval_s = excluded.interval_s,
			timeout_ms = excluded.timeout_ms, enabled = excluded.enabled, spec = excluded.spec`,
		m.ID, m.Name, m.Kind, int64(m.Interval/time.Second), m.Timeout.Milliseconds(), boolInt(m.Enabled), string(spec), m.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("store: save monitor %q: %w", m.ID, err)
	}
	return nil
}

// GetMonitor returns one monitor or monitor.ErrNotFound.
func (db *DB) GetMonitor(ctx context.Context, id string) (monitor.Monitor, error) {
	m, err := scanMonitor(db.QueryRowContext(ctx, monitorColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return monitor.Monitor{}, monitor.ErrNotFound
	}
	if err != nil {
		return monitor.Monitor{}, fmt.Errorf("store: get monitor %q: %w", id, err)
	}
	return m, nil
}

// ListMonitors returns every monitor, oldest first.
func (db *DB) ListMonitors(ctx context.Context) ([]monitor.Monitor, error) {
	rows, err := db.QueryContext(ctx, monitorColumns+` ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []monitor.Monitor
	for rows.Next() {
		m, err := scanMonitor(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan monitor: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate monitors: %w", err)
	}
	return out, nil
}

// CountMonitors returns the number of stored monitors.
func (db *DB) CountMonitors(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitors`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count monitors: %w", err)
	}
	return n, nil
}

// DeleteMonitor removes a monitor with its results and rollups, or returns monitor.ErrNotFound.
func (db *DB) DeleteMonitor(ctx context.Context, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete monitor %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM monitors WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete monitor %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return monitor.ErrNotFound
	}
	for _, q := range []string{`DELETE FROM monitor_results WHERE monitor_id = ?`, `DELETE FROM monitor_rollup_1h WHERE monitor_id = ?`, `DELETE FROM monitor_heartbeats WHERE monitor_id = ?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("store: delete monitor %q history: %w", id, err)
		}
	}
	return tx.Commit()
}
