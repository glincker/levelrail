package alerting

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HistoryStats returns the alert_history row count, its oldest entry (zero when empty) and its size in bytes when SQLite reports it.
func (db *DB) HistoryStats(ctx context.Context) (rows int64, oldest time.Time, bytes int64, err error) {
	var first string
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MIN(at), '') FROM alert_history`).Scan(&rows, &first); err != nil {
		return 0, time.Time{}, 0, fmt.Errorf("alerting: measure history: %w", err)
	}
	if first != "" {
		oldest, _ = time.Parse(time.RFC3339Nano, first)
	}
	var size sql.NullInt64
	if db.QueryRowContext(ctx, `SELECT SUM(pgsize) FROM dbstat WHERE name = 'alert_history'`).Scan(&size) == nil {
		bytes = size.Int64
	}
	return rows, oldest, bytes, nil
}

// CountHistoryBefore counts what PruneHistory would delete for the same cutoff.
func (db *DB) CountHistoryBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_history WHERE at < ?`, fmtTime(cutoff)).Scan(&n); err != nil {
		return 0, fmt.Errorf("alerting: count history to prune: %w", err)
	}
	return n, nil
}
