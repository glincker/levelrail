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

// VitalsCount is an aggregated increment for one rollup row.
type VitalsCount struct {
	App, Route, Metric, Device string
	Hour                       time.Time
	Bucket                     int
	Count                      int64
}

// AddVitalsCounts applies aggregated increments in one transaction; the caller buffers so this runs once per flush.
func (db *DB) AddVitalsCounts(ctx context.Context, rows []VitalsCount) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: add vitals counts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range rows {
		hour := c.Hour.UTC().Truncate(time.Hour).Format(time.RFC3339)
		_, err = tx.ExecContext(ctx, `INSERT INTO web_vitals_rollup (app, route, metric, hour, device, bucket, count) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(app, route, metric, hour, device, bucket) DO UPDATE SET count = count + excluded.count`,
			c.App, c.Route, c.Metric, hour, c.Device, c.Bucket, c.Count)
		if err != nil {
			return fmt.Errorf("store: add vitals counts: %w", err)
		}
	}
	return tx.Commit()
}

// VitalsRoutesSince lists up to limit distinct routes an app has rows for since a time.
func (db *DB) VitalsRoutesSince(ctx context.Context, app string, since time.Time, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT route FROM web_vitals_rollup WHERE app = ? AND hour >= ? LIMIT ?`,
		app, since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("store: vitals routes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, fmt.Errorf("store: vitals routes: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryVitals sums the counters since a time for the topRoutes busiest routes, grouped by route, metric and bucket.
func (db *DB) QueryVitals(ctx context.Context, app string, since time.Time, topRoutes int) ([]VitalsBucketCount, error) {
	cut := since.UTC().Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, `SELECT route, metric, bucket, SUM(count) FROM web_vitals_rollup
		WHERE app = ? AND hour >= ? AND route IN (
			SELECT route FROM web_vitals_rollup WHERE app = ? AND hour >= ?
			GROUP BY route ORDER BY SUM(count) DESC, route LIMIT ?)
		GROUP BY route, metric, bucket`, app, cut, app, cut, topRoutes)
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

// QueryVitalsOverall sums the counters since a time across every route, so the app-wide totals
// are not skewed by the top-routes limit that QueryVitals applies. Route is left empty.
func (db *DB) QueryVitalsOverall(ctx context.Context, app string, since time.Time) ([]VitalsBucketCount, error) {
	rows, err := db.QueryContext(ctx, `SELECT metric, bucket, SUM(count) FROM web_vitals_rollup
		WHERE app = ? AND hour >= ? GROUP BY metric, bucket`, app, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: query vitals overall: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []VitalsBucketCount
	for rows.Next() {
		var c VitalsBucketCount
		if err := rows.Scan(&c.Metric, &c.Bucket, &c.Count); err != nil {
			return nil, fmt.Errorf("store: query vitals overall: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PruneVitalsBatch deletes at most limit counters older than the cutoff so one call never holds the write lock long.
func (db *DB) PruneVitalsBatch(ctx context.Context, before time.Time, limit int) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM web_vitals_rollup WHERE rowid IN (
		SELECT rowid FROM web_vitals_rollup WHERE hour < ? LIMIT ?)`, before.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return 0, fmt.Errorf("store: prune vitals: %w", err)
	}
	return res.RowsAffected()
}

// DeleteAppVitals removes an app's beacon key and rollups when the app is deleted.
func (db *DB) DeleteAppVitals(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_vitals_config WHERE app = ?`, app); err != nil {
		return fmt.Errorf("store: delete vitals config for %q: %w", app, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM web_vitals_rollup WHERE app = ?`, app); err != nil {
		return fmt.Errorf("store: delete vitals rollups for %q: %w", app, err)
	}
	return nil
}
