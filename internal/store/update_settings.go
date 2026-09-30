package store

import (
	"context"
	"fmt"
)

// UpdateSettings is the platform-wide self-update configuration row
// (migrations/0258_update_settings.sql): which release channel to
// compare the running build against, and whether
// internal/updatecheck.Scheduler should run its periodic check at all.
// There is always exactly one of these (id = 1), seeded by the
// migration, so GetUpdateSettings never returns a not-found error.
type UpdateSettings struct {
	// Channel is one of "stable", "beta", or "edge" (internal/upgrade's
	// Channel constants). Defaults to "stable": GET /api/v1/updates
	// compares against GitHub's latest non-prerelease release, unchanged
	// from this table's own pre-existing behavior.
	Channel string
	// AutoUpdateEnabled gates internal/updatecheck.Scheduler's periodic
	// check. False by default: no background GitHub polling happens
	// until an operator opts in.
	AutoUpdateEnabled bool
}

// GetUpdateSettings returns the single update_settings row. Always
// succeeds against a migrated database: the migration itself inserts the
// row (id = 1), the same "no not-found case" guarantee GetIngressSettings
// already documents for its own singleton row.
func (db *DB) GetUpdateSettings(ctx context.Context) (UpdateSettings, error) {
	var (
		s                 UpdateSettings
		autoUpdateEnabled int
	)
	err := db.QueryRowContext(ctx, `
		SELECT channel, auto_update_enabled
		FROM update_settings
		WHERE id = 1
	`).Scan(&s.Channel, &autoUpdateEnabled)
	if err != nil {
		return UpdateSettings{}, fmt.Errorf("store: get update settings: %w", err)
	}
	s.AutoUpdateEnabled = autoUpdateEnabled != 0
	return s, nil
}

// UpdateUpdateSettings replaces the single update_settings row in full,
// the same "whole record always written as a whole" convention
// UpdateIngressSettings already establishes for its own singleton row.
// Validation (channel is one of the recognized values) is the caller's
// responsibility, matching UpdateIngressSettings's own division of
// labor between store and API layer.
func (db *DB) UpdateUpdateSettings(ctx context.Context, s UpdateSettings) error {
	autoUpdateEnabled := 0
	if s.AutoUpdateEnabled {
		autoUpdateEnabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE update_settings
		SET channel = ?, auto_update_enabled = ?
		WHERE id = 1
	`, s.Channel, autoUpdateEnabled)
	if err != nil {
		return fmt.Errorf("store: update update settings: %w", err)
	}
	return nil
}
