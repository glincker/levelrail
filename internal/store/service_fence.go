package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
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

// fenceScopeService is the fence_epoch key shared by every service placement token.
const fenceScopeService = "service"

type fenceQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// nextFenceEpoch advances the counter of scope and returns the new value. Tokens never go backwards,
// not even across a delete and recreate. floor lifts the counter to at least that value.
func nextFenceEpoch(ctx context.Context, q fenceQuerier, scope string, floor int64) (uint64, error) {
	var v int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO fence_epoch (key, value) VALUES (?, MAX(1, ?))
		ON CONFLICT(key) DO UPDATE SET value = MAX(value + 1, excluded.value)
		RETURNING value`, scope, floor).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("store: advance fence epoch %s: %w", scope, err)
	}
	return fenceTokenValue(v), nil
}

// NextNodeFenceToken returns the next cleanup token of a node. It never goes down, and the wall clock
// floor keeps it above the millisecond tokens older control planes already sent.
func (db *DB) NextNodeFenceToken(ctx context.Context, nodeID string) (uint64, error) {
	return nextFenceEpoch(ctx, db, "node:"+nodeID, time.Now().UnixMilli())
}

// withServiceFenceBump runs one placement update and stamps the row with the next epoch in one transaction.
func (db *DB) withServiceFenceBump(ctx context.Context, name string, setNode bool, nodeID string) (uint64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: bump fence token of %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	tok, err := nextFenceEpoch(ctx, tx, fenceScopeService, 0)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE desired_services SET node_id = CASE WHEN ? THEN ? ELSE node_id END, fence_token = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, setNode, nodeID, tok, name)
	if err != nil {
		return 0, fmt.Errorf("store: bump fence token of %s: %w", name, err)
	}
	if n, rerr := res.RowsAffected(); rerr != nil || n == 0 {
		return 0, ErrServiceNotFound
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: bump fence token of %s: %w", name, err)
	}
	return tok, nil
}

// BumpServiceFenceToken moves the token to the next global epoch and returns it, for lease grants and
// instance creation. A move does the same inside UpdateServiceNode.
func (db *DB) BumpServiceFenceToken(ctx context.Context, name string) (uint64, error) {
	tok, err := db.withServiceFenceBump(ctx, name, false, "")
	if errors.Is(err, ErrServiceNotFound) {
		return 0, fmt.Errorf("store: bump fence token of %s: service not found", name)
	}
	return tok, err
}

// fenceTokenValue clamps a stored counter to the unsigned wire type; the column is never negative.
func fenceTokenValue(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v) //nolint:gosec // checked non-negative above
}
