package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrBuildCacheSettingNotFound is returned when an app scope has no build cache setting.
var ErrBuildCacheSettingNotFound = errors.New("store: build cache setting not found")

// BuildCacheSetting points an app's BuildKit remote cache at a storage
// destination. AppName is empty for the global default.
type BuildCacheSetting struct {
	AppName       string
	TargetID      string
	Enabled       bool
	Mode          string
	LastBuildAt   string
	LastResult    string
	LastWarning   string
	LastClearedAt string
	UpdatedAt     string
}

const buildCacheCols = `app_name, target_id, enabled, mode, last_build_at, last_result, last_warning, last_cleared_at, updated_at`

func scanBuildCacheSetting(sc rowScanner) (BuildCacheSetting, error) {
	var s BuildCacheSetting
	var enabled int
	err := sc.Scan(&s.AppName, &s.TargetID, &enabled, &s.Mode, &s.LastBuildAt, &s.LastResult, &s.LastWarning, &s.LastClearedAt, &s.UpdatedAt)
	s.Enabled = enabled != 0
	return s, err
}

// UpsertBuildCacheSetting creates or updates the setting for s.AppName, keeping build history fields.
func (db *DB) UpsertBuildCacheSetting(ctx context.Context, s BuildCacheSetting) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO build_cache_settings (app_name, target_id, enabled, mode, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET target_id = excluded.target_id, enabled = excluded.enabled,
			mode = excluded.mode, updated_at = excluded.updated_at
	`, s.AppName, s.TargetID, boolInt(s.Enabled), s.Mode, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: upsert build cache setting %q: %w", s.AppName, err)
	}
	return nil
}

// GetBuildCacheSetting returns the setting for an app scope (empty means global).
func (db *DB) GetBuildCacheSetting(ctx context.Context, appName string) (BuildCacheSetting, error) {
	s, err := scanBuildCacheSetting(db.QueryRowContext(ctx,
		`SELECT `+buildCacheCols+` FROM build_cache_settings WHERE app_name = ?`, appName))
	if errors.Is(err, sql.ErrNoRows) {
		return BuildCacheSetting{}, ErrBuildCacheSettingNotFound
	}
	if err != nil {
		return BuildCacheSetting{}, fmt.Errorf("store: get build cache setting %q: %w", appName, err)
	}
	return s, nil
}

// ListBuildCacheSettings returns every setting, global first.
func (db *DB) ListBuildCacheSettings(ctx context.Context) ([]BuildCacheSetting, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+buildCacheCols+` FROM build_cache_settings ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list build cache settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BuildCacheSetting
	for rows.Next() {
		s, err := scanBuildCacheSetting(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan build cache setting: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate build cache settings: %w", err)
	}
	return out, nil
}

// DeleteBuildCacheSetting removes the setting for an app scope.
func (db *DB) DeleteBuildCacheSetting(ctx context.Context, appName string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM build_cache_settings WHERE app_name = ?`, appName)
	if err != nil {
		return fmt.Errorf("store: delete build cache setting %q: %w", appName, err)
	}
	return rowsAffectedOrNotFound(res, ErrBuildCacheSettingNotFound, "delete build cache setting %q", appName)
}

// RecordBuildCacheResult stores the outcome of the latest build that used the
// cache configured at scope (the row an app resolved to). A missing row is not an error.
func (db *DB) RecordBuildCacheResult(ctx context.Context, scope, at, result, warning string) error {
	if _, err := db.ExecContext(ctx, `UPDATE build_cache_settings SET last_build_at = ?, last_result = ?, last_warning = ?
		WHERE app_name = ?`, at, result, warning, scope); err != nil {
		return fmt.Errorf("store: record build cache result %q: %w", scope, err)
	}
	return nil
}

// RecordBuildCacheCleared stamps a cache clear on an app scope.
func (db *DB) RecordBuildCacheCleared(ctx context.Context, scope, at string) error {
	if _, err := db.ExecContext(ctx, `UPDATE build_cache_settings SET last_cleared_at = ? WHERE app_name = ?`, at, scope); err != nil {
		return fmt.Errorf("store: record build cache cleared %q: %w", scope, err)
	}
	return nil
}

// SetDeployAttemptCacheWarning records why a build ran without its remote cache.
func (db *DB) SetDeployAttemptCacheWarning(ctx context.Context, id, warning string) error {
	if _, err := db.ExecContext(ctx, `UPDATE deploy_attempts SET cache_warning = ? WHERE id = ?`, warning, id); err != nil {
		return fmt.Errorf("store: set deploy attempt %q cache warning: %w", id, err)
	}
	return nil
}

// ListDeployAttemptCacheWarnings maps attempt id to its cache warning for a service.
func (db *DB) ListDeployAttemptCacheWarnings(ctx context.Context, serviceName string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, cache_warning FROM deploy_attempts WHERE service_name = ? AND cache_warning != ''`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list cache warnings for %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, w string
		if err := rows.Scan(&id, &w); err != nil {
			return nil, fmt.Errorf("store: scan cache warning: %w", err)
		}
		out[id] = w
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate cache warnings: %w", err)
	}
	return out, nil
}
