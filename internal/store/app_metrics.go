package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrAppMetricsTargetNotFound is returned when an app has no scrape target.
var ErrAppMetricsTargetNotFound = errors.New("store: app metrics target not found")

// AppMetricsTarget is the port and path an app serves its own metrics on.
type AppMetricsTarget struct {
	App       string
	Port      int
	Path      string
	LastError string
	UpdatedAt string
}

// AppMetricSample is one stored value of a scraped series.
type AppMetricSample struct {
	Name   string
	Labels string
	At     time.Time
	Value  float64
}

// SetAppMetricsTarget creates or replaces an app's scrape target.
func (db *DB) SetAppMetricsTarget(ctx context.Context, t AppMetricsTarget) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_metrics_targets (app, port, path, last_error, updated_at) VALUES (?, ?, ?, '', ?)
		ON CONFLICT(app) DO UPDATE SET port = excluded.port, path = excluded.path, last_error = '', updated_at = excluded.updated_at`,
		t.App, t.Port, t.Path, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set metrics target for %q: %w", t.App, err)
	}
	return nil
}

// RecordAppMetricsScrapeError keeps the last scrape failure so the dashboard can show it.
func (db *DB) RecordAppMetricsScrapeError(ctx context.Context, app, msg string) error {
	_, err := db.ExecContext(ctx, `UPDATE app_metrics_targets SET last_error = ? WHERE app = ?`, msg, app)
	return err
}

// GetAppMetricsTarget returns an app's scrape target.
func (db *DB) GetAppMetricsTarget(ctx context.Context, app string) (AppMetricsTarget, error) {
	var t AppMetricsTarget
	err := db.QueryRowContext(ctx, `SELECT app, port, path, last_error, updated_at FROM app_metrics_targets WHERE app = ?`, app).
		Scan(&t.App, &t.Port, &t.Path, &t.LastError, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AppMetricsTarget{}, ErrAppMetricsTargetNotFound
	}
	return t, err
}

// DeleteAppMetricsTarget stops scraping an app; its stored samples age out by retention.
func (db *DB) DeleteAppMetricsTarget(ctx context.Context, app string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM app_metrics_targets WHERE app = ?`, app)
	return err
}

// ListAppMetricsTargets returns every configured scrape target.
func (db *DB) ListAppMetricsTargets(ctx context.Context) ([]AppMetricsTarget, error) {
	rows, err := db.QueryContext(ctx, `SELECT app, port, path, last_error, updated_at FROM app_metrics_targets ORDER BY app`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AppMetricsTarget
	for rows.Next() {
		var t AppMetricsTarget
		if err := rows.Scan(&t.App, &t.Port, &t.Path, &t.LastError, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ErrAppMetricsRowCap is returned when an app's stored samples are at the cap; the scrape is dropped.
var ErrAppMetricsRowCap = errors.New("store: app metrics stored row limit reached")

// InsertAppMetricSamples stores one scrape in a single transaction, only while the
// target still exists, so a scrape racing a delete leaves nothing behind.
func (db *DB) InsertAppMetricSamples(ctx context.Context, app string, samples []AppMetricSample, maxRows int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_metrics_targets WHERE app = ?`, app).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrAppMetricsTargetNotFound
	}
	var stored int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_metric_samples
		WHERE series_id IN (SELECT id FROM app_metric_series WHERE app = ?)`, app).Scan(&stored); err != nil {
		return err
	}
	if stored+len(samples) > maxRows {
		return ErrAppMetricsRowCap
	}
	series, err := tx.PrepareContext(ctx, `INSERT INTO app_metric_series (app, name, labels) VALUES (?, ?, ?)
		ON CONFLICT(app, name, labels) DO UPDATE SET name = excluded.name RETURNING id`)
	if err != nil {
		return err
	}
	defer func() { _ = series.Close() }()
	sample, err := tx.PrepareContext(ctx, `INSERT INTO app_metric_samples (series_id, ts, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = sample.Close() }()
	for _, s := range samples {
		var id int64
		if err := series.QueryRowContext(ctx, app, s.Name, s.Labels).Scan(&id); err != nil {
			return fmt.Errorf("store: upsert metric series for %q: %w", app, err)
		}
		if _, err := sample.ExecContext(ctx, id, s.At.UnixMilli(), s.Value); err != nil {
			return fmt.Errorf("store: insert metric sample for %q: %w", app, err)
		}
	}
	return tx.Commit()
}

// ListAppMetricNames returns the distinct series names stored for an app since a time.
func (db *DB) ListAppMetricNames(ctx context.Context, app string, since time.Time, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT r.name FROM app_metric_series r
		WHERE r.app = ? AND EXISTS (SELECT 1 FROM app_metric_samples s WHERE s.series_id = r.id AND s.ts >= ?)
		ORDER BY r.name LIMIT ?`, app, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// QueryAppMetricSamples returns at most limit of the newest samples in a window, oldest first.
func (db *DB) QueryAppMetricSamples(ctx context.Context, app, name string, from, to time.Time, limit int) ([]AppMetricSample, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.name, r.labels, s.ts, s.value
		FROM app_metric_samples s JOIN app_metric_series r ON r.id = s.series_id
		WHERE r.app = ? AND r.name = ? AND s.ts >= ? AND s.ts <= ? ORDER BY s.ts DESC LIMIT ?`,
		app, name, from.UnixMilli(), to.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]AppMetricSample, 0, min(limit, 1024))
	for rows.Next() {
		var s AppMetricSample
		var ts int64
		if err := rows.Scan(&s.Name, &s.Labels, &ts, &s.Value); err != nil {
			return nil, err
		}
		s.At = time.UnixMilli(ts).UTC()
		out = append(out, s)
	}
	slices.Reverse(out)
	return out, rows.Err()
}

// PruneAppMetricSamples deletes samples older than before and series left empty.
func (db *DB) PruneAppMetricSamples(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_metric_samples WHERE ts < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	_, err = db.ExecContext(ctx, `DELETE FROM app_metric_series WHERE NOT EXISTS (SELECT 1 FROM app_metric_samples s WHERE s.series_id = app_metric_series.id)`)
	return n, err
}
