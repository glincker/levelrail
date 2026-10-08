package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ImageAutoUpdate is one app's opt-in to redeploy when its image tag moves
// (migrations/0377_image_auto_updates.sql). A missing row means disabled.
type ImageAutoUpdate struct {
	ServiceName   string
	Enabled       bool
	LastCheckedAt *time.Time
	LastResult    string
	// WebhookHash is the hash of the registry webhook token, empty when none is set.
	WebhookHash string
}

// SetImageAutoUpdate enables or disables auto-update for serviceName.
func (db *DB) SetImageAutoUpdate(ctx context.Context, serviceName string, enabled bool) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO image_auto_updates (service_name, enabled, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at
	`, serviceName, enabled, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set image auto-update for %q: %w", serviceName, err)
	}
	return nil
}

// GetImageAutoUpdate returns serviceName's row, or a disabled zero value when none exists.
func (db *DB) GetImageAutoUpdate(ctx context.Context, serviceName string) (ImageAutoUpdate, error) {
	u := ImageAutoUpdate{ServiceName: serviceName}
	var enabled int
	var checked sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT enabled, last_checked_at, last_result, webhook_hash FROM image_auto_updates WHERE service_name = ?
	`, serviceName).Scan(&enabled, &checked, &u.LastResult, &u.WebhookHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, nil
	}
	if err != nil {
		return ImageAutoUpdate{}, fmt.Errorf("store: get image auto-update for %q: %w", serviceName, err)
	}
	u.Enabled = enabled != 0
	if u.LastCheckedAt, err = parseTimePtr(checked); err != nil {
		return ImageAutoUpdate{}, fmt.Errorf("store: get image auto-update for %q: %w", serviceName, err)
	}
	return u, nil
}

// ListEnabledImageAutoUpdates returns every app that opted in.
func (db *DB) ListEnabledImageAutoUpdates(ctx context.Context) ([]ImageAutoUpdate, error) {
	rows, err := db.QueryContext(ctx, `SELECT service_name FROM image_auto_updates WHERE enabled = 1 ORDER BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list image auto-updates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ImageAutoUpdate
	for rows.Next() {
		u := ImageAutoUpdate{Enabled: true}
		if err := rows.Scan(&u.ServiceName); err != nil {
			return nil, fmt.Errorf("store: scan image auto-update: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list image auto-updates: %w", err)
	}
	return out, nil
}

// RecordImageAutoUpdateCheck stores the outcome of the latest check.
func (db *DB) RecordImageAutoUpdateCheck(ctx context.Context, serviceName, result string, at time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO image_auto_updates (service_name, enabled, last_checked_at, last_result, updated_at) VALUES (?, 0, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET last_checked_at = excluded.last_checked_at, last_result = excluded.last_result
	`, serviceName, at.UTC().Format(time.RFC3339), result, at.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: record image auto-update check for %q: %w", serviceName, err)
	}
	return nil
}

// SetImageAutoUpdateWebhookHash stores the hash of serviceName's registry webhook token; empty clears it.
func (db *DB) SetImageAutoUpdateWebhookHash(ctx context.Context, serviceName, hash string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO image_auto_updates (service_name, enabled, webhook_hash, updated_at) VALUES (?, 0, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET webhook_hash = excluded.webhook_hash, updated_at = excluded.updated_at
	`, serviceName, hash, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set image auto-update webhook for %q: %w", serviceName, err)
	}
	return nil
}
