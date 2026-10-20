package telemetry

import (
	"context"
	"fmt"
)

const vacuumFreeRatio = 0.25

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
	if pages == 0 || float64(free)/float64(pages) <= vacuumFreeRatio {
		return nil
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("telemetry: vacuum: %w", err)
	}
	return nil
}
