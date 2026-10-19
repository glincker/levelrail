package store

import (
	"context"
	"fmt"
	"time"
)

// ListPendingNodeJoinTokens returns tokens that are unused and unexpired at now, newest first.
func (db *DB) ListPendingNodeJoinTokens(ctx context.Context, now time.Time) ([]NodeJoinToken, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, token_hash, created_at, expires_at, used_at, purpose, COALESCE(node_id, '')
		FROM node_join_tokens WHERE used_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC
	`, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("store: list pending node join tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeJoinToken
	for rows.Next() {
		t, err := scanNodeJoinToken(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node join token: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list pending node join tokens: %w", err)
	}
	return out, nil
}

// RevokePendingNodeJoinTokens expires pending tokens at now: just id when non-empty, otherwise all.
// A token already used or expired is left untouched. It returns how many were revoked.
func (db *DB) RevokePendingNodeJoinTokens(ctx context.Context, id string, now time.Time) (int64, error) {
	q := `UPDATE node_join_tokens SET expires_at = ? WHERE used_at IS NULL AND expires_at > ?`
	args := []any{formatTime(now.Add(-time.Second)), formatTime(now)}
	if id != "" {
		q += ` AND id = ?`
		args = append(args, id)
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("store: revoke pending node join tokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: revoke pending node join tokens: rows affected: %w", err)
	}
	return n, nil
}
