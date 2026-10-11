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
	// MaxPreviews overrides the platform per-app cap when above zero.
	MaxPreviews int
	// MemoryLimit and CPULimit override the platform preview resource
	// defaults when set.
	MemoryLimit string
	CPULimit    float64
	// IdleSleepMinutes overrides the platform idle sleep default when above
	// zero.
	IdleSleepMinutes int
	// DatabaseStrategy is one of the PreviewDatabaseStrategy* values.
	DatabaseStrategy string
	// SeedDatabase names the database whose latest backup seeds a preview
	// under the seed strategy.
	SeedDatabase string
	// AllowForkSecrets lets an approved or allowed fork preview receive the
	// app's secret environment variables; off by default.
	AllowForkSecrets bool
	// GateBasicAuth puts HTTP basic auth (GateUsername) in front of every
	// preview domain of the app.
	GateBasicAuth bool
	GateUsername  string
	// AllowIndexing lets search engines index preview URLs; off by default.
	AllowIndexing bool
}

// Preview database strategies: what database a preview talks to.
const (
	PreviewDatabaseStrategyNone   = "none"
	PreviewDatabaseStrategyShared = "shared"
	PreviewDatabaseStrategyFresh  = "fresh"
	PreviewDatabaseStrategySeed   = "seed"
)

// ValidPreviewDatabaseStrategy reports whether s is a known strategy.
func ValidPreviewDatabaseStrategy(s string) bool {
	switch s {
	case PreviewDatabaseStrategyNone, PreviewDatabaseStrategyShared, PreviewDatabaseStrategyFresh, PreviewDatabaseStrategySeed:
		return true
	}
	return false
}

const previewSettingsColumns = `app_name, on_limit, allow_fork_previews, ttl_hours, updated_at,
	max_previews, memory_limit, cpu_limit, idle_sleep_minutes, database_strategy, seed_database,
	allow_fork_secrets, gate_basic_auth, gate_username, allow_indexing`

func scanPreviewSettings(scan func(dest ...any) error) (PreviewAppSettings, error) {
	var (
		s                                      PreviewAppSettings
		allowFork, forkSecrets, gate, indexing int
	)
	err := scan(&s.AppName, &s.OnLimit, &allowFork, &s.TTLHours, &s.UpdatedAt,
		&s.MaxPreviews, &s.MemoryLimit, &s.CPULimit, &s.IdleSleepMinutes, &s.DatabaseStrategy, &s.SeedDatabase,
		&forkSecrets, &gate, &s.GateUsername, &indexing)
	s.AllowForkPreviews, s.AllowForkSecrets, s.GateBasicAuth, s.AllowIndexing = allowFork != 0, forkSecrets != 0, gate != 0, indexing != 0
	return s, err
}

// DefaultPreviewAppSettings is the policy an app has before anyone saves one.
func DefaultPreviewAppSettings(appName string) PreviewAppSettings {
	return PreviewAppSettings{AppName: appName, OnLimit: PreviewOnLimitEvictOldest, DatabaseStrategy: PreviewDatabaseStrategyNone}
}

// GetPreviewAppSettings returns appName's saved policy, or the defaults
// when none has been saved.
func (db *DB) GetPreviewAppSettings(ctx context.Context, appName string) (PreviewAppSettings, error) {
	s, err := scanPreviewSettings(db.QueryRowContext(ctx, `
		SELECT `+previewSettingsColumns+` FROM preview_app_settings WHERE app_name = ?
	`, appName).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPreviewAppSettings(appName), nil
	}
	if err != nil {
		return PreviewAppSettings{}, fmt.Errorf("store: get preview settings for %q: %w", appName, err)
	}
	return s, nil
}

// ListPreviewAppSettings returns every saved policy keyed by app name.
func (db *DB) ListPreviewAppSettings(ctx context.Context) (map[string]PreviewAppSettings, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+previewSettingsColumns+` FROM preview_app_settings`)
	if err != nil {
		return nil, fmt.Errorf("store: list preview settings: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := make(map[string]PreviewAppSettings)
	for rows.Next() {
		s, err := scanPreviewSettings(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan preview settings row: %w", err)
		}
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
	if s.DatabaseStrategy == "" {
		s.DatabaseStrategy = PreviewDatabaseStrategyNone
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO preview_app_settings (`+previewSettingsColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET
			on_limit = excluded.on_limit,
			allow_fork_previews = excluded.allow_fork_previews,
			ttl_hours = excluded.ttl_hours,
			updated_at = excluded.updated_at,
			max_previews = excluded.max_previews,
			memory_limit = excluded.memory_limit,
			cpu_limit = excluded.cpu_limit,
			idle_sleep_minutes = excluded.idle_sleep_minutes,
			database_strategy = excluded.database_strategy,
			seed_database = excluded.seed_database,
			allow_fork_secrets = excluded.allow_fork_secrets,
			gate_basic_auth = excluded.gate_basic_auth,
			gate_username = excluded.gate_username,
			allow_indexing = excluded.allow_indexing
	`, s.AppName, s.OnLimit, boolToInt(s.AllowForkPreviews), s.TTLHours, s.UpdatedAt,
		s.MaxPreviews, s.MemoryLimit, s.CPULimit, s.IdleSleepMinutes, s.DatabaseStrategy, s.SeedDatabase,
		boolToInt(s.AllowForkSecrets), boolToInt(s.GateBasicAuth), s.GateUsername, boolToInt(s.AllowIndexing))
	if err != nil {
		return fmt.Errorf("store: save preview settings for %q: %w", s.AppName, err)
	}
	return nil
}
