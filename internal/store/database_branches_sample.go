package store

import (
	"context"
	"fmt"
)

// SetDatabaseBranchClone records how the data was copied and why a faster path was skipped.
func (db *DB) SetDatabaseBranchClone(ctx context.Context, id, clonePath, reason, now string) error {
	res, err := db.ExecContext(ctx, `UPDATE database_branches SET clone_path = ?, clone_path_reason = ?, updated_at = ? WHERE id = ?`, clonePath, reason, now, id)
	if err != nil {
		return fmt.Errorf("store: set database branch clone %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDatabaseBranchNotFound
	}
	return nil
}

// SetDatabaseBranchSize stores a sampled volume size.
func (db *DB) SetDatabaseBranchSize(ctx context.Context, id string, bytes int64, now string) error {
	res, err := db.ExecContext(ctx, `UPDATE database_branches SET size_bytes = ?, size_measured_at = ? WHERE id = ?`, max(bytes, 0), now, id)
	if err != nil {
		return fmt.Errorf("store: set database branch size %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDatabaseBranchNotFound
	}
	return nil
}
