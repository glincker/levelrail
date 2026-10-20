package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ServerProfileSetting is the persisted server profile choice. Set is false
// when the operator has never chosen.
type ServerProfileSetting struct {
	Profile   string
	Set       bool
	UpdatedAt time.Time
	UpdatedBy string
}

// GetServerProfile returns the saved profile, or the zero value when none exists.
func (db *DB) GetServerProfile(ctx context.Context) (ServerProfileSetting, error) {
	var profile, updatedAt, updatedBy string
	err := db.QueryRowContext(ctx, `SELECT profile, updated_at, updated_by FROM server_profile_settings WHERE id = 1`).
		Scan(&profile, &updatedAt, &updatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerProfileSetting{}, nil
	}
	if err != nil {
		return ServerProfileSetting{}, fmt.Errorf("store: get server profile: %w", err)
	}
	t, _ := time.Parse(time.RFC3339Nano, updatedAt)
	return ServerProfileSetting{Profile: profile, Set: true, UpdatedAt: t, UpdatedBy: updatedBy}, nil
}

// SetServerProfile saves the profile choice.
func (db *DB) SetServerProfile(ctx context.Context, profile, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO server_profile_settings (id, profile, updated_at, updated_by) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET profile = excluded.profile, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		profile, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set server profile: %w", err)
	}
	return nil
}

// CountEnabledPlugins counts plugins whose containers the supervisor runs.
func (db *DB) CountEnabledPlugins(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugins WHERE state = 'enabled'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count enabled plugins: %w", err)
	}
	return n, nil
}

// CountScheduledBackups counts databases and service volumes with a backup schedule.
func (db *DB) CountScheduledBackups(ctx context.Context) (int, error) {
	dbs, err := db.ListScheduledDatabases(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count scheduled backups: %w", err)
	}
	vols, err := db.ListScheduledServiceVolumes(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count scheduled backups: %w", err)
	}
	return len(dbs) + len(vols), nil
}
