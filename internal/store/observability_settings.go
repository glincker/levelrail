package store

import (
	"context"
	"fmt"
)

// ObservabilitySettings is the single platform-wide row: the external
// Grafana (or other dashboard) URL an operator links out to from the
// metrics charts. Empty means unset: no link is shown.
type ObservabilitySettings struct {
	ExternalDashboardURL string
}

// GetObservabilitySettings returns the single observability_settings row.
// Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetObservabilitySettings(ctx context.Context) (ObservabilitySettings, error) {
	var url string
	err := db.QueryRowContext(ctx, `
		SELECT external_dashboard_url FROM observability_settings WHERE id = 1
	`).Scan(&url)
	if err != nil {
		return ObservabilitySettings{}, fmt.Errorf("store: get observability settings: %w", err)
	}
	return ObservabilitySettings{ExternalDashboardURL: url}, nil
}

// UpdateObservabilitySettings replaces the single row's URL, the same
// whole-record convention UpdateCloudflareTunnelSettings uses.
func (db *DB) UpdateObservabilitySettings(ctx context.Context, s ObservabilitySettings) error {
	_, err := db.ExecContext(ctx, `
		UPDATE observability_settings SET external_dashboard_url = ? WHERE id = 1
	`, s.ExternalDashboardURL)
	if err != nil {
		return fmt.Errorf("store: update observability settings: %w", err)
	}
	return nil
}
