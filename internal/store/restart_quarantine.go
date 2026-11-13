package store

import (
	"context"
	"fmt"
	"time"
)

// RestartQuarantine is one container held by the crash-loop breaker.
type RestartQuarantine struct {
	Container     string
	AppName       string
	Restarts      int
	QuarantinedAt time.Time
}

// SaveRestartQuarantine records or refreshes a quarantined container.
func (db *DB) SaveRestartQuarantine(ctx context.Context, q RestartQuarantine) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO restart_quarantine (container, app_name, restarts, quarantined_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(container) DO UPDATE SET app_name = excluded.app_name, restarts = excluded.restarts`,
		q.Container, q.AppName, q.Restarts, q.QuarantinedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save restart quarantine %q: %w", q.Container, err)
	}
	return nil
}

// DeleteRestartQuarantine forgets a container, a missing one is not an error.
func (db *DB) DeleteRestartQuarantine(ctx context.Context, container string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM restart_quarantine WHERE container = ?`, container); err != nil {
		return fmt.Errorf("store: delete restart quarantine %q: %w", container, err)
	}
	return nil
}

// ListRestartQuarantine returns every held container.
func (db *DB) ListRestartQuarantine(ctx context.Context) ([]RestartQuarantine, error) {
	rows, err := db.QueryContext(ctx, `SELECT container, app_name, restarts, quarantined_at FROM restart_quarantine ORDER BY container`)
	if err != nil {
		return nil, fmt.Errorf("store: list restart quarantine: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RestartQuarantine
	for rows.Next() {
		var q RestartQuarantine
		var at string
		if err := rows.Scan(&q.Container, &q.AppName, &q.Restarts, &at); err != nil {
			return nil, fmt.Errorf("store: scan restart quarantine: %w", err)
		}
		q.QuarantinedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, q)
	}
	return out, rows.Err()
}

// PruneRestartQuarantine drops rows whose app no longer exists and returns how many went.
func (db *DB) PruneRestartQuarantine(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM restart_quarantine WHERE app_name NOT IN (SELECT name FROM desired_services)`)
	if err != nil {
		return 0, fmt.Errorf("store: prune restart quarantine: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
