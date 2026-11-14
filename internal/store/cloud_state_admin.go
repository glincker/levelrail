package store

import (
	"context"
	"fmt"
	"time"
)

// ListCloudStateQuarantineForStack returns the unresolved rows of a stack across cloud accounts.
func (db *DB) ListCloudStateQuarantineForStack(ctx context.Context, stack string) ([]CloudStateQuarantine, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, account, fence, reason, created_at FROM cloud_state_quarantine
		WHERE stack = ? AND resolved_at IS NULL ORDER BY id`, stack)
	if err != nil {
		return nil, fmt.Errorf("store: list cloud state quarantine for stack: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CloudStateQuarantine
	for rows.Next() {
		q := CloudStateQuarantine{Stack: stack}
		var created string
		if err := rows.Scan(&q.ID, &q.Account, &q.Fence, &q.Reason, &created); err != nil {
			return nil, fmt.Errorf("store: scan cloud state quarantine: %w", err)
		}
		q.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, q)
	}
	return out, rows.Err()
}

// ResolveCloudStateQuarantineForStack clears one row by id and stack, returning its account.
func (db *DB) ResolveCloudStateQuarantineForStack(ctx context.Context, stack string, id int64, now time.Time) (string, error) {
	var account string
	err := db.QueryRowContext(ctx, `SELECT account FROM cloud_state_quarantine WHERE id = ? AND stack = ? AND resolved_at IS NULL`, id, stack).Scan(&account)
	if err != nil {
		return "", ErrCloudStateQuarantineNotFound
	}
	if err := db.ResolveCloudStateQuarantine(ctx, account, stack, id, now); err != nil {
		return "", err
	}
	return account, nil
}
