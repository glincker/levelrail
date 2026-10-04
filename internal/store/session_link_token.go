package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SessionLinkToken is one outstanding session-link mint: a short-lived,
// single-use token that establishes a session for whoever minted it,
// without a password. TokenHash is the only form of the token ever
// persisted. Abilities and DisplayName are a snapshot taken at mint
// time, since a PrincipalTypeToken principal has no live user record
// this could be re-resolved from at consume time, unlike a session's own
// user row.
type SessionLinkToken struct {
	ID            string
	PrincipalType string
	PrincipalID   string
	Abilities     []string
	DisplayName   string
	TokenHash     string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
}

// ErrSessionLinkTokenNotFound is returned by GetSessionLinkTokenByHash
// when no row matches.
var ErrSessionLinkTokenNotFound = errors.New("store: session link token not found")

// SaveSessionLinkToken inserts a new session-link-token row.
func (db *DB) SaveSessionLinkToken(ctx context.Context, t SessionLinkToken) error {
	abilitiesJSON, err := json.Marshal(nonNilSlice(t.Abilities))
	if err != nil {
		return fmt.Errorf("store: marshal abilities for session link token %q: %w", t.ID, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO session_link_tokens (id, principal_type, principal_id, abilities, display_name, token_hash, created_at, expires_at, used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.PrincipalType, t.PrincipalID, string(abilitiesJSON), t.DisplayName, t.TokenHash, formatTime(t.CreatedAt), formatTime(t.ExpiresAt), formatTimePtr(t.UsedAt))
	if err != nil {
		return fmt.Errorf("store: save session link token %q: %w", t.ID, err)
	}
	return nil
}

// GetSessionLinkTokenByHash returns the session-link-token row matching
// hash, regardless of expiry or used state: deciding whether the token
// is currently usable is the caller's job.
func (db *DB) GetSessionLinkTokenByHash(ctx context.Context, hash string) (*SessionLinkToken, error) {
	var (
		t             SessionLinkToken
		abilitiesJSON string
		createdAt     string
		expiresAt     string
		usedAt        sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, principal_type, principal_id, abilities, display_name, token_hash, created_at, expires_at, used_at
		FROM session_link_tokens WHERE token_hash = ?
	`, hash).Scan(&t.ID, &t.PrincipalType, &t.PrincipalID, &abilitiesJSON, &t.DisplayName, &t.TokenHash, &createdAt, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionLinkTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get session link token by hash: %w", err)
	}

	if err := json.Unmarshal([]byte(abilitiesJSON), &t.Abilities); err != nil {
		return nil, fmt.Errorf("store: parse session link token abilities: %w", err)
	}
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse session link token created_at: %w", err)
	}
	t.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse session link token expires_at: %w", err)
	}
	t.UsedAt, err = parseTimePtr(usedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse session link token used_at: %w", err)
	}
	return &t, nil
}

// ErrSessionLinkTokenAlreadyUsed is returned by ClaimSessionLinkToken
// when the token was already used by a concurrent request.
var ErrSessionLinkTokenAlreadyUsed = errors.New("store: session link token already used")

// ClaimSessionLinkToken atomically marks the named token used, only if
// it hasn't been already: the WHERE clause and rows-affected check make
// this the single point two concurrent requests holding the same token
// race on, so at most one can ever establish a session from it. Same
// shape as ClaimPasswordResetToken.
func (db *DB) ClaimSessionLinkToken(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE session_link_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND used_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("store: claim session link token %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: claim session link token %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrSessionLinkTokenAlreadyUsed
	}
	return nil
}
