package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecordEmailProviderResult stamps a send outcome on a provider: a success sets
// last_ok_at and clears the error kind, a failure sets the error kind only.
func (db *DB) RecordEmailProviderResult(ctx context.Context, id string, ok bool, errorKind string, at time.Time) error {
	var res sql.Result
	var err error
	if ok {
		res, err = db.ExecContext(ctx, `UPDATE email_providers SET last_ok_at = ?, last_error_kind = '' WHERE id = ?`, formatProviderTime(at), id)
	} else {
		res, err = db.ExecContext(ctx, `UPDATE email_providers SET last_error_kind = ? WHERE id = ?`, errorKind, id)
	}
	if err != nil {
		return fmt.Errorf("store: record email provider result: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEmailProviderNotFound
	}
	return nil
}
