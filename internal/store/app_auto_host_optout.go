package store

import (
	"context"
	"fmt"
)

// GetAppAutoHostOptOut reports whether app opted out of its automatic hostname (migrations/3178).
func (db *DB) GetAppAutoHostOptOut(ctx context.Context, app string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_auto_host_optout WHERE app_name = ?`, app).Scan(&n); err != nil {
		return false, fmt.Errorf("store: get auto host opt-out: %w", err)
	}
	return n > 0, nil
}

// SetAppAutoHostOptOut records or clears app's opt-out.
func (db *DB) SetAppAutoHostOptOut(ctx context.Context, app string, optOut bool) error {
	q := `DELETE FROM app_auto_host_optout WHERE app_name = ?`
	if optOut {
		q = `INSERT OR IGNORE INTO app_auto_host_optout (app_name) VALUES (?)`
	}
	if _, err := db.ExecContext(ctx, q, app); err != nil {
		return fmt.Errorf("store: set auto host opt-out: %w", err)
	}
	return nil
}

// ListAppAutoHostOptOuts returns every opted-out app name.
func (db *DB) ListAppAutoHostOptOuts(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT app_name FROM app_auto_host_optout`)
	if err != nil {
		return nil, fmt.Errorf("store: list auto host opt-outs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var app string
		if err := rows.Scan(&app); err != nil {
			return nil, fmt.Errorf("store: scan auto host opt-out: %w", err)
		}
		out[app] = true
	}
	return out, rows.Err()
}
