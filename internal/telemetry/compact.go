package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
)

const vacuumFreeRatio = 0.25

// Defaults for the size cap headroom and the row estimate used when dbstat is missing.
const (
	defaultCapTargetPercent = 85
	estSampleRowBytes       = 40
	estRollupRowBytes       = 65
)

var capTargetSource atomic.Pointer[func() int]

// SetCapTargetPercentSource makes prune aim for fn percent of the cap, so a saved setting applies to the next run.
func SetCapTargetPercentSource(fn func() int) { capTargetSource.Store(&fn) }

func currentCapTargetPercent() int {
	if fn := capTargetSource.Load(); fn != nil {
		if p := (*fn)(); p >= 1 && p <= 100 {
			return p
		}
	}
	return defaultCapTargetPercent
}

// metricBytes counts the metric tables and their indexes only, so logs and spans never trigger metric pruning.
func (db *DB) metricBytes(ctx context.Context) (int64, error) {
	var size sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT SUM(pgsize) FROM dbstat WHERE name IN
		(SELECT name FROM sqlite_master WHERE tbl_name IN ('metric_samples', 'metric_rollups'))`).Scan(&size)
	if err == nil && size.Valid {
		return size.Int64, nil
	}
	// dbstat is optional in some builds, so fall back to rows times measured bytes per row.
	var samples, rollups int64
	if err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM metric_samples), (SELECT COUNT(*) FROM metric_rollups)`).Scan(&samples, &rollups); err != nil {
		return 0, fmt.Errorf("telemetry: count metric rows: %w", err)
	}
	return samples*estSampleRowBytes + rollups*estRollupRowBytes, nil
}

// FileBytes returns the size of the database file including free pages.
func (db *DB) FileBytes(ctx context.Context) (int64, error) {
	var pages, size int64
	if err := db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
		return 0, fmt.Errorf("telemetry: page_count: %w", err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&size); err != nil {
		return 0, fmt.Errorf("telemetry: page_size: %w", err)
	}
	return pages * size, nil
}

// Compact prunes past retention and the size cap, then returns freed pages to the filesystem when more than a quarter of the file is free.
func (db *DB) Compact(ctx context.Context) error {
	if _, err := db.Prune(ctx, db.now()); err != nil {
		return err
	}
	var pages, free int64
	if err := db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
		return fmt.Errorf("telemetry: page_count: %w", err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&free); err != nil {
		return fmt.Errorf("telemetry: freelist_count: %w", err)
	}
	var mode int
	if err := db.QueryRowContext(ctx, `PRAGMA auto_vacuum`).Scan(&mode); err == nil && mode == 2 && free > 0 {
		// Incremental databases return free pages cheaply without a full rewrite.
		if _, err := db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
			return fmt.Errorf("telemetry: incremental vacuum: %w", err)
		}
		return nil
	}
	if pages == 0 || float64(free)/float64(pages) <= vacuumFreeRatio {
		return nil
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("telemetry: vacuum: %w", err)
	}
	return nil
}
