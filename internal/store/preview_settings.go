package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Preview limit policies applied when an app's preview cap is full.
const (
	PreviewOnLimitEvictOldest = "evict_oldest"
	PreviewOnLimitReject      = "reject"
)

// PreviewAppSettings is one app's preview policy: what happens at the
// concurrency cap, whether fork pull requests may deploy, and an optional
// per-app TTL in hours (0 means the control plane default).
type PreviewAppSettings struct {
	AppName           string
	OnLimit           string
	AllowForkPreviews bool
	TTLHours          int
	UpdatedAt         string
}

// DefaultPreviewAppSettings is the policy an app has before anyone saves one.
func DefaultPreviewAppSettings(appName string) PreviewAppSettings {
	return PreviewAppSettings{AppName: appName, OnLimit: PreviewOnLimitEvictOldest}
}

// GetPreviewAppSettings returns appName's saved policy, or the defaults
// when none has been saved.
func (db *DB) GetPreviewAppSettings(ctx context.Context, appName string) (PreviewAppSettings, error) {
	var (
		s     PreviewAppSettings
		allow int
	)
	err := db.QueryRowContext(ctx, `
		SELECT app_name, on_limit, allow_fork_previews, ttl_hours, updated_at
		FROM preview_app_settings WHERE app_name = ?
	`, appName).Scan(&s.AppName, &s.OnLimit, &allow, &s.TTLHours, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPreviewAppSettings(appName), nil
	}
	if err != nil {
		return PreviewAppSettings{}, fmt.Errorf("store: get preview settings for %q: %w", appName, err)
	}
	s.AllowForkPreviews = allow != 0
	return s, nil
}

// ListPreviewAppSettings returns every saved policy keyed by app name.
func (db *DB) ListPreviewAppSettings(ctx context.Context) (map[string]PreviewAppSettings, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT app_name, on_limit, allow_fork_previews, ttl_hours, updated_at FROM preview_app_settings
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list preview settings: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := make(map[string]PreviewAppSettings)
	for rows.Next() {
		var (
			s     PreviewAppSettings
			allow int
		)
		if err := rows.Scan(&s.AppName, &s.OnLimit, &allow, &s.TTLHours, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan preview settings row: %w", err)
		}
		s.AllowForkPreviews = allow != 0
		out[s.AppName] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate preview settings rows: %w", err)
	}
	return out, nil
}

// SavePreviewAppSettings upserts an app's policy.
func (db *DB) SavePreviewAppSettings(ctx context.Context, s PreviewAppSettings) error {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO preview_app_settings (app_name, on_limit, allow_fork_previews, ttl_hours, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET
			on_limit = excluded.on_limit,
			allow_fork_previews = excluded.allow_fork_previews,
			ttl_hours = excluded.ttl_hours,
			updated_at = excluded.updated_at
	`, s.AppName, s.OnLimit, boolToInt(s.AllowForkPreviews), s.TTLHours, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save preview settings for %q: %w", s.AppName, err)
	}
	return nil
}
