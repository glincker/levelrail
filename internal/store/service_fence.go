package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ServiceFenceToken returns the placement token of a service, 0 when the service does not exist.
func (db *DB) ServiceFenceToken(ctx context.Context, name string) (uint64, error) {
	var tok int64
	err := db.QueryRowContext(ctx, `SELECT fence_token FROM desired_services WHERE name = ?`, name).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: read fence token of %s: %w", name, err)
	}
	return fenceTokenValue(tok), nil
}

// BumpServiceFenceToken raises the token by one and returns it, for lease grants and instance creation.
// A move bumps it inside UpdateServiceNode's own statement.
func (db *DB) BumpServiceFenceToken(ctx context.Context, name string) (uint64, error) {
	var tok int64
	err := db.QueryRowContext(ctx, `UPDATE desired_services SET fence_token = fence_token + 1 WHERE name = ? RETURNING fence_token`, name).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: bump fence token of %s: service not found", name)
	}
	if err != nil {
		return 0, fmt.Errorf("store: bump fence token of %s: %w", name, err)
	}
	return fenceTokenValue(tok), nil
}

// fenceTokenValue clamps a stored counter to the unsigned wire type; the column is never negative.
func fenceTokenValue(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v) //nolint:gosec // checked non-negative above
}
