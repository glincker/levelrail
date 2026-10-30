package store

import "context"

// CountApps returns how many apps exist, or 0 when the count cannot be read.
func (db *DB) CountApps(ctx context.Context) int {
	var n int
	if err := db.reader().QueryRowContext(ctx, `SELECT COUNT(*) FROM apps`).Scan(&n); err != nil {
		return 0
	}
	return n
}
