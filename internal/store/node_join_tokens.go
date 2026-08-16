package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NodeJoinToken is a one-time credential (ADR 003: "the control plane
// issues a one-time join token, agent exchanges it for a client
// certificate") an operator pastes into a new node's enrollment
// command. Only the hash is ever stored here, the same shape
// APIToken/api_tokens (0007) already established: the plaintext is
// generated once by internal/api, shown to the operator once, and never
// recoverable from this table again.
type NodeJoinToken struct {
	ID        string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	// UsedAt is nil until the token is exchanged for a node enrollment
	// (TASKS.md 3.2). Single-use is enforced by MarkNodeJoinTokenUsed's
	// atomic conditional UPDATE, not by application code checking this
	// field first: two concurrent exchange attempts racing on the same
	// token must not both succeed, the identical concurrency hazard
	// store.CreateUser's own doc comment documents for first-run
	// registration.
	UsedAt *time.Time
}

// ErrNodeJoinTokenNotFound is returned by GetNodeJoinTokenByHash and
// MarkNodeJoinTokenUsed when no token matches.
var ErrNodeJoinTokenNotFound = errors.New("store: node join token not found")

// ErrNodeJoinTokenAlreadyUsed is MarkNodeJoinTokenUsed's failure mode
// when the token has already been exchanged once.
var ErrNodeJoinTokenAlreadyUsed = errors.New("store: node join token already used")

// SaveNodeJoinToken inserts a new join token row.
func (db *DB) SaveNodeJoinToken(ctx context.Context, t NodeJoinToken) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_join_tokens (id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, t.ID, t.TokenHash, formatTime(t.CreatedAt), formatTime(t.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: save node join token %q: %w", t.ID, err)
	}
	return nil
}

// GetNodeJoinTokenByHash returns the token row matching hash, used or
// not, expired or not: deciding whether a token is currently redeemable
// is the caller's job (internal/api's enrollment handler, TASKS.md 3.2),
// matching GetAPITokenByHash's identical "pure lookup, not a decision"
// separation.
func (db *DB) GetNodeJoinTokenByHash(ctx context.Context, hash string) (*NodeJoinToken, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, token_hash, created_at, expires_at, used_at
		FROM node_join_tokens WHERE token_hash = ?
	`, hash)
	t, err := scanNodeJoinToken(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNodeJoinTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get node join token by hash: %w", err)
	}
	return t, nil
}

// MarkNodeJoinTokenUsed atomically marks id as exchanged, failing with
// ErrNodeJoinTokenAlreadyUsed if it already was: the conditional
// `WHERE used_at IS NULL` is what makes this safe under concurrent
// exchange attempts on the same token, the database deciding which
// caller wins rather than a check-then-act race in application code.
func (db *DB) MarkNodeJoinTokenUsed(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE node_join_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND used_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("store: mark node join token %q used: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: mark node join token %q used: rows affected: %w", id, err)
	}
	if n == 1 {
		return nil
	}

	// No rows changed: either this token never existed, or it was
	// already used (possibly by a concurrent caller that won the race).
	// Distinguish the two with a follow-up read, the same pattern
	// RevokeAPIToken already applies to its own conditional UPDATE.
	existing, err := db.getNodeJoinTokenByID(ctx, id)
	if err != nil {
		return fmt.Errorf("store: mark node join token %q used: %w", id, err)
	}
	if existing == nil {
		return ErrNodeJoinTokenNotFound
	}
	return ErrNodeJoinTokenAlreadyUsed
}

func (db *DB) getNodeJoinTokenByID(ctx context.Context, id string) (*NodeJoinToken, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, token_hash, created_at, expires_at, used_at
		FROM node_join_tokens WHERE id = ?
	`, id)
	t, err := scanNodeJoinToken(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanNodeJoinToken(scan func(dest ...any) error) (*NodeJoinToken, error) {
	var (
		t                  NodeJoinToken
		createdAt, expires string
		usedAt             sql.NullString
	)
	if err := scan(&t.ID, &t.TokenHash, &createdAt, &expires, &usedAt); err != nil {
		return nil, err
	}
	var err error
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	t.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	t.UsedAt, err = parseTimePtr(usedAt)
	if err != nil {
		return nil, fmt.Errorf("parse used_at: %w", err)
	}
	return &t, nil
}
