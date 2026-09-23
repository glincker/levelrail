package store

import (
	"context"
	"database/sql"
	"fmt"
)

// GetDashboardURL returns the configured public dashboard URL, or "" when unset.
func (db *DB) GetDashboardURL(ctx context.Context) (string, error) {
	var u sql.NullString
	err := db.QueryRowContext(ctx, `SELECT dashboard_url FROM ingress_settings WHERE id = 1`).Scan(&u)
	if err != nil {
		return "", fmt.Errorf("store: get dashboard url: %w", err)
	}
	return u.String, nil
}

// SetDashboardURL stores the public dashboard URL; "" clears it.
func (db *DB) SetDashboardURL(ctx context.Context, u string) error {
	_, err := db.ExecContext(ctx, `UPDATE ingress_settings SET dashboard_url = ? WHERE id = 1`,
		sql.NullString{String: u, Valid: u != ""})
	if err != nil {
		return fmt.Errorf("store: set dashboard url: %w", err)
	}
	return nil
}
