package store

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

// InsertResults stores a batch and folds it into the hourly rollups in one
// transaction. A duplicate (monitor, second) is skipped and not double counted.
func (db *DB) InsertResults(ctx context.Context, results []monitor.Result) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: insert monitor results: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stored := make([]monitor.Result, 0, len(results))
	for _, r := range results {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO monitor_results (monitor_id, at, location, ok, latency_ms, code, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			r.MonitorID, r.At, monitor.Location, boolInt(r.OK), r.LatencyMS, r.Code, r.Reason)
		if err != nil {
			return fmt.Errorf("store: insert monitor result: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			stored = append(stored, r)
		}
	}
	for _, ru := range monitor.Aggregate(stored) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO monitor_rollup_1h (monitor_id, bucket, total, failed, lat_sum, lat_max)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (monitor_id, bucket) DO UPDATE SET total = total + excluded.total,
				failed = failed + excluded.failed, lat_sum = lat_sum + excluded.lat_sum,
				lat_max = MAX(lat_max, excluded.lat_max)`,
			ru.MonitorID, ru.Bucket, ru.Total, ru.Failed, ru.LatSum, ru.LatMax); err != nil {
			return fmt.Errorf("store: upsert monitor rollup: %w", err)
		}
	}
	return tx.Commit()
}

// ListResults returns raw results at or after since, newest first.
func (db *DB) ListResults(ctx context.Context, id string, since int64, limit int) ([]monitor.Result, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT at, ok, latency_ms, code, reason FROM monitor_results
		WHERE monitor_id = ? AND at >= ? ORDER BY at DESC LIMIT ?`, id, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list monitor results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []monitor.Result
	for rows.Next() {
		r := monitor.Result{MonitorID: id}
		var ok int
		if err := rows.Scan(&r.At, &ok, &r.LatencyMS, &r.Code, &r.Reason); err != nil {
			return nil, fmt.Errorf("store: scan monitor result: %w", err)
		}
		r.OK = ok != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRollups returns hour buckets at or after sinceBucket, oldest first.
func (db *DB) ListRollups(ctx context.Context, id string, sinceBucket int64, limit int) ([]monitor.Rollup, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT bucket, total, failed, lat_sum, lat_max FROM monitor_rollup_1h
		WHERE monitor_id = ? AND bucket >= ? ORDER BY bucket LIMIT ?`, id, sinceBucket, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list monitor rollups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []monitor.Rollup
	for rows.Next() {
		r := monitor.Rollup{MonitorID: id}
		if err := rows.Scan(&r.Bucket, &r.Total, &r.Failed, &r.LatSum, &r.LatMax); err != nil {
			return nil, fmt.Errorf("store: scan monitor rollup: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneResults deletes up to limit raw rows older than before and returns how many went.
func (db *DB) PruneResults(ctx context.Context, id string, before int64, limit int) (int, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM monitor_results WHERE (monitor_id, at, location) IN (
			SELECT monitor_id, at, location FROM monitor_results WHERE monitor_id = ? AND at < ? LIMIT ?)`,
		id, before, limit)
	if err != nil {
		return 0, fmt.Errorf("store: prune monitor results: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneRollups deletes up to limit hour buckets older than before and returns how many went.
func (db *DB) PruneRollups(ctx context.Context, id string, before int64, limit int) (int, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM monitor_rollup_1h WHERE (monitor_id, bucket) IN (
			SELECT monitor_id, bucket FROM monitor_rollup_1h WHERE monitor_id = ? AND bucket < ? LIMIT ?)`,
		id, before, limit)
	if err != nil {
		return 0, fmt.Errorf("store: prune monitor rollups: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
