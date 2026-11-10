package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AppBuildSettings is one app's persistent build options
// (migrations/3229_app_build_settings.sql).
type AppBuildSettings struct {
	AppName   string
	Target    string
	NoCache   bool
	Provider  string
	UpdatedAt string
}

// GetAppBuildSettings returns the app's settings; a missing row is the zero value with found false.
func (db *DB) GetAppBuildSettings(ctx context.Context, appName string) (AppBuildSettings, bool, error) {
	s := AppBuildSettings{AppName: appName}
	var noCache int
	err := db.QueryRowContext(ctx, `SELECT target, no_cache, provider, updated_at FROM app_build_settings WHERE app_name = ?`, appName).
		Scan(&s.Target, &noCache, &s.Provider, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, fmt.Errorf("store: get build settings for %q: %w", appName, err)
	}
	s.NoCache = noCache != 0
	return s, true, nil
}

// SaveAppBuildSettings upserts the app's settings.
func (db *DB) SaveAppBuildSettings(ctx context.Context, s AppBuildSettings) error {
	if s.UpdatedAt == "" {
		s.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	noCache := 0
	if s.NoCache {
		noCache = 1
	}
	// The EXISTS guard keeps a save racing an app delete from leaving an orphan row.
	res, err := db.ExecContext(ctx, `
		INSERT INTO app_build_settings (app_name, target, no_cache, provider, updated_at)
		SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM desired_services WHERE name = ?)
		ON CONFLICT(app_name) DO UPDATE SET target = excluded.target, no_cache = excluded.no_cache,
			provider = excluded.provider, updated_at = excluded.updated_at
	`, s.AppName, s.Target, noCache, s.Provider, s.UpdatedAt, s.AppName)
	if err != nil {
		return fmt.Errorf("store: save build settings for %q: %w", s.AppName, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrServiceNotFound
	}
	return nil
}
