package store

import (
	"context"
	"database/sql"
)

// ReadQueryContext runs a standalone SELECT on the read pool (or the writer
// when no pool exists) for packages outside store.
func (db *DB) ReadQueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.reader().QueryContext(ctx, query, args...)
}

// ReadQueryRowContext is the single-row form of ReadQueryContext.
func (db *DB) ReadQueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.reader().QueryRowContext(ctx, query, args...)
}
