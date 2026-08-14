package telemetry

import (
	"context"
	"fmt"
	"time"
)

// Sample is one metric reading at one point in time for one resource
// (e.g. resource_id "service:web", metric "cpu_percent").
type Sample struct {
	ResourceID string
	Metric     string
	Timestamp  time.Time
	Value      float64
}

// WriteSamples inserts every sample in one transaction: a collection
// tick writes many samples (several metrics per running container) and
// they should land atomically, not partially, if the process is
// interrupted mid-write. Writing the same (resource, metric, timestamp)
// twice replaces the value rather than erroring, since a collector that
// retries a tick after a transient failure should be safe to just run
// again, not required to first check what already landed.
func (db *DB) WriteSamples(ctx context.Context, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin write: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metric_samples (resource_id, metric, ts, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (resource_id, metric, ts) DO UPDATE SET value = excluded.value
	`)
	if err != nil {
		return fmt.Errorf("telemetry: prepare write: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, s := range samples {
		if _, err := stmt.ExecContext(ctx, s.ResourceID, s.Metric, s.Timestamp.Unix(), s.Value); err != nil {
			return fmt.Errorf("telemetry: write sample %s/%s@%s: %w", s.ResourceID, s.Metric, s.Timestamp, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit write: %w", err)
	}
	return nil
}

// Query returns every sample for resourceID/metric with a timestamp in
// [from, to], oldest first. An empty (nil) result and a nil error both
// mean "no samples in this range," not an error: a resource with no
// activity yet, or a range before collection started, is a valid
// observed state, the same convention docker.InspectByName's "not
// found is not an error" doc comment already establishes elsewhere in
// this codebase.
func (db *DB) Query(ctx context.Context, resourceID, metric string, from, to time.Time) ([]Sample, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ts, value FROM metric_samples
		WHERE resource_id = ? AND metric = ? AND ts BETWEEN ? AND ?
		ORDER BY ts ASC
	`, resourceID, metric, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("telemetry: query %s/%s: %w", resourceID, metric, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Sample
	for rows.Next() {
		var tsUnix int64
		var value float64
		if err := rows.Scan(&tsUnix, &value); err != nil {
			return nil, fmt.Errorf("telemetry: scan sample row: %w", err)
		}
		out = append(out, Sample{
			ResourceID: resourceID,
			Metric:     metric,
			Timestamp:  time.Unix(tsUnix, 0).UTC(),
			Value:      value,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate sample rows: %w", err)
	}
	return out, nil
}

// Retain deletes every sample older than the given cutoff, returning how
// many rows were removed. The documented default retention (15 days) is
// a caller concern (the collector or a scheduled sweep decides the
// cutoff), not hardcoded here, per the house "no hardcoded thresholds"
// rule.
func (db *DB) Retain(ctx context.Context, cutoff time.Time) (deleted int64, err error) {
	res, err := db.ExecContext(ctx, `DELETE FROM metric_samples WHERE ts < ?`, cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("telemetry: retention sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: retention sweep rows affected: %w", err)
	}
	return n, nil
}
