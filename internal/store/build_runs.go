package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Keys of build_limit_settings. An absent key means the env default applies.
const (
	BuildLimitMaxConcurrent  = "max_concurrent"
	BuildLimitTimeoutSeconds = "timeout_seconds"
	BuildLimitMemoryMB       = "memory_mb"
	BuildLimitCPUMillis      = "cpu_millis"
	buildRunsKeepPerApp      = 200
	buildRunsListMaxRows     = 500
)

// BuildRun is one finished build as stored.
type BuildRun struct {
	App         string
	Tag         string
	StartedAt   time.Time
	DurationMS  int64
	QueueWaitMS int64
	Status      string
	CachedSteps int
	TotalSteps  int
	Error       string
	NodeID      string
	Platform    string
	ImageBytes  int64
}

// RecordBuildRun appends one finished build and trims the app's history.
func (db *DB) RecordBuildRun(ctx context.Context, r BuildRun) error {
	_, err := db.ExecContext(ctx, `INSERT INTO build_runs
		(app_name, tag, started_at, duration_ms, queue_wait_ms, status, cached_steps, total_steps, error, node_id, platform, image_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.App, r.Tag, r.StartedAt.UTC().Format(time.RFC3339Nano), r.DurationMS, r.QueueWaitMS,
		r.Status, r.CachedSteps, r.TotalSteps, r.Error, r.NodeID, r.Platform, r.ImageBytes)
	if err != nil {
		return fmt.Errorf("store: record build run: %w", err)
	}
	_, err = db.ExecContext(ctx, `DELETE FROM build_runs WHERE app_name = ? AND id NOT IN
		(SELECT id FROM build_runs WHERE app_name = ? ORDER BY id DESC LIMIT ?)`, r.App, r.App, buildRunsKeepPerApp)
	if err != nil {
		return fmt.Errorf("store: trim build runs: %w", err)
	}
	return nil
}

// ListBuildRuns returns an app's newest builds first, at most limit.
func (db *DB) ListBuildRuns(ctx context.Context, app string, limit int) ([]BuildRun, error) {
	if limit <= 0 || limit > buildRunsListMaxRows {
		limit = buildRunsListMaxRows
	}
	return db.queryBuildRuns(ctx, `WHERE app_name = ? ORDER BY id DESC LIMIT ?`, app, limit)
}

// ListAllBuildRuns returns the newest builds of every app, newest first.
func (db *DB) ListAllBuildRuns(ctx context.Context, limit int) ([]BuildRun, error) {
	if limit <= 0 || limit > buildRunsListMaxRows {
		limit = buildRunsListMaxRows
	}
	return db.queryBuildRuns(ctx, `ORDER BY id DESC LIMIT ?`, limit)
}

// ListBuildAttempts returns the newest deploy attempts that built from source
// (plain image and compose deploys build nothing), newest first.
func (db *DB) ListBuildAttempts(ctx context.Context, limit int) ([]DeployAttempt, error) {
	if limit <= 0 || limit > buildRunsListMaxRows {
		limit = buildRunsListMaxRows
	}
	return db.queryDeployAttempts(ctx, "build", `source NOT IN (?, ?) ORDER BY started_at DESC LIMIT ?`,
		DeployAttemptSourceImage, DeployAttemptSourceCompose, limit)
}

func (db *DB) queryBuildRuns(ctx context.Context, clause string, args ...any) ([]BuildRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT app_name, tag, started_at, duration_ms, queue_wait_ms, status, cached_steps, total_steps, error, node_id, platform, image_bytes
		FROM build_runs `+clause, args...) //nolint:gosec // clause is a package-internal constant
	if err != nil {
		return nil, fmt.Errorf("store: list build runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BuildRun
	for rows.Next() {
		var r BuildRun
		var started string
		if err := rows.Scan(&r.App, &r.Tag, &started, &r.DurationMS, &r.QueueWaitMS, &r.Status, &r.CachedSteps, &r.TotalSteps, &r.Error, &r.NodeID, &r.Platform, &r.ImageBytes); err != nil {
			return nil, fmt.Errorf("store: scan build run: %w", err)
		}
		r.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		out = append(out, r)
	}
	return out, rows.Err()
}

// BuildLimitAppKey is the build_limit_settings key of one app's override of key.
func BuildLimitAppKey(app, key string) string { return "app/" + app + "/" + key }

// GetBuildLimitSetting returns a stored override.
func (db *DB) GetBuildLimitSetting(ctx context.Context, key string) (int64, bool, error) {
	var v int64
	err := db.QueryRowContext(ctx, `SELECT value FROM build_limit_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: get build limit setting: %w", err)
	}
	return v, true, nil
}

// SetBuildLimitSetting upserts an override.
func (db *DB) SetBuildLimitSetting(ctx context.Context, key string, value int64, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO build_limit_settings (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		key, value, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set build limit setting: %w", err)
	}
	return nil
}

// ClearBuildLimitSetting removes an override so the env default applies again.
func (db *DB) ClearBuildLimitSetting(ctx context.Context, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM build_limit_settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("store: clear build limit setting: %w", err)
	}
	return nil
}
