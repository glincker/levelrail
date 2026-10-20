package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrFeatureFlagNotFound is returned by GetFeatureFlag, GetFeatureFlagByKey,
// UpdateFeatureFlag, and DeleteFeatureFlag when id/key doesn't match any
// row.
var ErrFeatureFlagNotFound = errors.New("store: feature flag not found")

// FeatureFlag is a boolean (plus an optional gradual rollout percentage)
// an app's own code reads live at runtime, never baked into a container
// at create time. See migrations/0065_feature_flags.sql for why Key is
// globally unique rather than scoped to ServiceName.
type FeatureFlag struct {
	ID                string
	Key               string
	Name              string
	Description       string
	ServiceName       string
	Enabled           bool
	RolloutPercentage int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SaveFeatureFlag inserts a new feature flag row. ID is minted by the
// caller (internal/api), the same "generate before the INSERT" pattern
// SaveScheduledTask's own doc comment establishes.
func (db *DB) SaveFeatureFlag(ctx context.Context, f FeatureFlag) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO feature_flags (id, key, name, description, service_name, enabled, rollout_percentage, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID, f.Key, f.Name, f.Description, f.ServiceName, f.Enabled, f.RolloutPercentage, formatTime(f.CreatedAt), formatTime(f.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save feature flag %q: %w", f.ID, err)
	}
	return nil
}

// GetFeatureFlag returns the feature flag with this ID, or
// ErrFeatureFlagNotFound.
func (db *DB) GetFeatureFlag(ctx context.Context, id string) (FeatureFlag, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, key, name, description, service_name, enabled, rollout_percentage, created_at, updated_at
		FROM feature_flags
		WHERE id = ?
	`, id)
	f, err := scanFeatureFlag(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureFlag{}, ErrFeatureFlagNotFound
	}
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("store: get feature flag %q: %w", id, err)
	}
	return *f, nil
}

// GetFeatureFlagByKey returns the feature flag with this Key, or
// ErrFeatureFlagNotFound. This is what handleEvaluateFeatureFlag looks up
// by, since the evaluate endpoint's URL carries no app name (see this
// table's own migration comment).
func (db *DB) GetFeatureFlagByKey(ctx context.Context, key string) (FeatureFlag, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, key, name, description, service_name, enabled, rollout_percentage, created_at, updated_at
		FROM feature_flags
		WHERE key = ?
	`, key)
	f, err := scanFeatureFlag(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureFlag{}, ErrFeatureFlagNotFound
	}
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("store: get feature flag by key %q: %w", key, err)
	}
	return *f, nil
}

// ListFeatureFlagsForService returns every feature flag owned by
// serviceName, oldest first, the same creation-order convention
// ListScheduledTasksForService already uses.
func (db *DB) ListFeatureFlagsForService(ctx context.Context, serviceName string) ([]FeatureFlag, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, key, name, description, service_name, enabled, rollout_percentage, created_at, updated_at
		FROM feature_flags
		WHERE service_name = ?
		ORDER BY created_at
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list feature flags for %q: %w", serviceName, err)
	}
	return scanFeatureFlags(rows)
}

// UpdateFeatureFlag replaces name/description/enabled/rollout_percentage
// for an existing row: a full-replace contract, the same shape
// UpdateScheduledTask uses. It never touches Key or ServiceName (an
// existing flag cannot be reassigned to a different key or app; delete
// and recreate instead). Returns ErrFeatureFlagNotFound if id doesn't
// exist.
func (db *DB) UpdateFeatureFlag(ctx context.Context, id, name, description string, enabled bool, rolloutPercentage int, updatedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE feature_flags
		SET name = ?, description = ?, enabled = ?, rollout_percentage = ?, updated_at = ?
		WHERE id = ?
	`, name, description, enabled, rolloutPercentage, formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update feature flag %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrFeatureFlagNotFound, "update feature flag %q", id)
}

// DeleteFeatureFlag removes a feature flag row. Returns
// ErrFeatureFlagNotFound if id doesn't exist.
func (db *DB) DeleteFeatureFlag(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM feature_flags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete feature flag %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrFeatureFlagNotFound, "delete feature flag %q", id)
}

func scanFeatureFlags(rows *sql.Rows) ([]FeatureFlag, error) {
	defer func() {
		_ = rows.Close()
	}()

	var out []FeatureFlag
	for rows.Next() {
		f, err := scanFeatureFlag(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan feature flag row: %w", err)
		}
		out = append(out, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate feature flag rows: %w", err)
	}
	return out, nil
}

func scanFeatureFlag(scan func(dest ...any) error) (*FeatureFlag, error) {
	var (
		f                    FeatureFlag
		enabled              int
		createdAt, updatedAt string
	)
	if err := scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.ServiceName, &enabled, &f.RolloutPercentage, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	f.Enabled = enabled != 0

	var err error
	f.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	f.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &f, nil
}
