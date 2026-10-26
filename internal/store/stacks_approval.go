package store

import (
	"context"
	"fmt"
)

// SetPendingStackRevisionMeta keeps the non-secret apply options of a revision
// awaiting approval in result_json. The terminal status later replaces it.
func (db *DB) SetPendingStackRevisionMeta(ctx context.Context, stackID string, number int, meta []byte) error {
	res, err := db.ExecContext(ctx, `UPDATE stack_revisions SET result_json = ? WHERE stack_id = ? AND number = ? AND status = ?`,
		meta, stackID, number, StackRevPendingApproval)
	if err != nil {
		return fmt.Errorf("store: set pending revision meta: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStackRevisionNotFound
	}
	return nil
}
