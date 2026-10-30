package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RolloutPolicy is one app's saved rollout: block
// (migrations/3050_rollout_policies.sql). Empty fields inherit env defaults.
type RolloutPolicy struct {
	ServiceName    string
	MaxSurge       string
	MaxUnavailable string
	MinAvailable   string
}

// GetRolloutPolicy returns serviceName's rollout settings and whether any exist.
func (db *DB) GetRolloutPolicy(ctx context.Context, serviceName string) (RolloutPolicy, bool, error) {
	p := RolloutPolicy{ServiceName: serviceName}
	err := db.QueryRowContext(ctx, `
		SELECT max_surge, max_unavailable, min_available FROM rollout_policies WHERE service_name = ?
	`, serviceName).Scan(&p.MaxSurge, &p.MaxUnavailable, &p.MinAvailable)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, fmt.Errorf("store: get rollout policy for %q: %w", serviceName, err)
	}
	return p, true, nil
}

// SetRolloutPolicy replaces p.ServiceName's rollout settings.
func (db *DB) SetRolloutPolicy(ctx context.Context, p RolloutPolicy) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO rollout_policies (service_name, max_surge, max_unavailable, min_available, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET max_surge = excluded.max_surge,
			max_unavailable = excluded.max_unavailable, min_available = excluded.min_available,
			updated_at = excluded.updated_at
	`, p.ServiceName, p.MaxSurge, p.MaxUnavailable, p.MinAvailable, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set rollout policy for %q: %w", p.ServiceName, err)
	}
	return nil
}

// DeleteRolloutPolicy clears serviceName's rollout settings.
func (db *DB) DeleteRolloutPolicy(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM rollout_policies WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete rollout policy for %q: %w", serviceName, err)
	}
	return nil
}
