package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// VitalsConfig is one app's beacon key and enable switch.
type VitalsConfig struct {
	App       string
	PublicKey string
	Enabled   bool
}

// VitalsHit is one observation already mapped to its histogram bucket.
type VitalsHit struct {
	Metric string
	Bucket int
}

// VitalsBucketCount is a summed counter for one route, metric and bucket.
type VitalsBucketCount struct {
	Route  string
	Metric string
	Bucket int
	Count  int64
}

func newVitalsKey() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: vitals key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GetVitalsConfig returns the app's config; found is false when none was saved yet.
func (db *DB) GetVitalsConfig(ctx context.Context, app string) (cfg VitalsConfig, found bool, err error) {
	var en int
	err = db.QueryRowContext(ctx, `SELECT public_key, enabled FROM app_vitals_config WHERE app = ?`, app).Scan(&cfg.PublicKey, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return VitalsConfig{}, false, nil
	}
	if err != nil {
		return VitalsConfig{}, false, fmt.Errorf("store: get vitals config: %w", err)
	}
	cfg.App, cfg.Enabled = app, en != 0
	return cfg, true, nil
}

// SetVitalsEnabled creates the config with a fresh key when missing, then sets the switch.
func (db *DB) SetVitalsEnabled(ctx context.Context, app string, enabled bool) (VitalsConfig, error) {
	key, err := newVitalsKey()
	if err != nil {
		return VitalsConfig{}, err
	}
	en := 0
	if enabled {
		en = 1
	}
	_, err = db.ExecContext(ctx, `INSERT INTO app_vitals_config (app, public_key, enabled, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(app) DO UPDATE SET enabled = excluded.enabled`, app, key, en, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return VitalsConfig{}, fmt.Errorf("store: set vitals enabled: %w", err)
	}
	cfg, _, err := db.GetVitalsConfig(ctx, app)
	return cfg, err
}

// RotateVitalsKey replaces the public key, creating the config with the given switch when missing.
func (db *DB) RotateVitalsKey(ctx context.Context, app string, enabledIfNew bool) (VitalsConfig, error) {
	key, err := newVitalsKey()
	if err != nil {
		return VitalsConfig{}, err
	}
	en := 0
	if enabledIfNew {
		en = 1
	}
	_, err = db.ExecContext(ctx, `INSERT INTO app_vitals_config (app, public_key, enabled, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(app) DO UPDATE SET public_key = excluded.public_key`, app, key, en, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return VitalsConfig{}, fmt.Errorf("store: rotate vitals key: %w", err)
	}
	cfg, _, err := db.GetVitalsConfig(ctx, app)
	return cfg, err
}

// AddVitalsHits increments the hourly bucket counters in one transaction.
func (db *DB) AddVitalsHits(ctx context.Context, app, route, device string, at time.Time, hits []VitalsHit) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: add vitals hits: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	hour := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	for _, h := range hits {
		_, err = tx.ExecContext(ctx, `INSERT INTO web_vitals_rollup (app, route, metric, hour, device, bucket, count) VALUES (?, ?, ?, ?, ?, ?, 1)
			ON CONFLICT(app, route, metric, hour, device, bucket) DO UPDATE SET count = count + 1`, app, route, h.Metric, hour, device, h.Bucket)
		if err != nil {
			return fmt.Errorf("store: add vitals hits: %w", err)
		}
	}
	return tx.Commit()
}

// QueryVitals sums the counters since a time, grouped by route, metric and bucket.
func (db *DB) QueryVitals(ctx context.Context, app string, since time.Time) ([]VitalsBucketCount, error) {
	rows, err := db.QueryContext(ctx, `SELECT route, metric, bucket, SUM(count) FROM web_vitals_rollup
		WHERE app = ? AND hour >= ? GROUP BY route, metric, bucket`, app, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: query vitals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []VitalsBucketCount
	for rows.Next() {
		var c VitalsBucketCount
		if err := rows.Scan(&c.Route, &c.Metric, &c.Bucket, &c.Count); err != nil {
			return nil, fmt.Errorf("store: query vitals: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PruneVitals deletes counters older than the cutoff and returns how many rows went.
func (db *DB) PruneVitals(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM web_vitals_rollup WHERE hour < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("store: prune vitals: %w", err)
	}
	return res.RowsAffected()
}
