package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PasswordResetToken is one outstanding forgot-password reset attempt
// (migrations/0032_password_reset_tokens.sql). TokenHash is the only form
// of the token ever persisted, the same "hash, never the raw secret"
// convention APIToken.TokenHash already establishes.
type PasswordResetToken struct {
	ID        string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ErrPasswordResetTokenNotFound is returned by
// GetPasswordResetTokenByHash when no row matches: internal/api's
// reset-password handler treats this identically to an expired or
// already-used token (a generic, non-distinguishing rejection), it is
// exposed as its own sentinel here only because the store layer itself
// always distinguishes "no such row" from a real database error.
var ErrPasswordResetTokenNotFound = errors.New("store: password reset token not found")

// SavePasswordResetToken inserts a new reset-token row. IDs are minted
// by the caller (internal/api), the same "generate before the INSERT"
// pattern SaveBackupTarget's own doc comment establishes.
func (db *DB) SavePasswordResetToken(ctx context.Context, t PasswordResetToken) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (id, token_hash, created_at, expires_at, used_at)
		VALUES (?, ?, ?, ?, ?)
	`, t.ID, t.TokenHash, formatTime(t.CreatedAt), formatTime(t.ExpiresAt), formatTimePtr(t.UsedAt))
	if err != nil {
		return fmt.Errorf("store: save password reset token %q: %w", t.ID, err)
	}
	return nil
}

// GetPasswordResetTokenByHash returns the reset-token row matching hash,
// regardless of whether it's expired or already used: deciding "is this
// token currently usable" is the caller's job (internal/api's
// handleResetPassword), the same separation GetAPITokenByHash's own doc
// comment establishes for bearer tokens.
func (db *DB) GetPasswordResetTokenByHash(ctx context.Context, hash string) (*PasswordResetToken, error) {
	var (
		t         PasswordResetToken
		createdAt string
		expiresAt string
		usedAt    sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, token_hash, created_at, expires_at, used_at
		FROM password_reset_tokens WHERE token_hash = ?
	`, hash).Scan(&t.ID, &t.TokenHash, &createdAt, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPasswordResetTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get password reset token by hash: %w", err)
	}

	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse password reset token created_at: %w", err)
	}
	t.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse password reset token expires_at: %w", err)
	}
	t.UsedAt, err = parseTimePtr(usedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse password reset token used_at: %w", err)
	}
	return &t, nil
}

// MarkPasswordResetTokenUsed sets used_at on the named token, making it
// permanently unusable for a second reset attempt. Idempotent at the SQL
// level (a second call just re-sets used_at to a new timestamp), but
// internal/api's handleResetPassword only ever calls this once, right
// after a successful password change, and treats a row whose used_at is
// already set as invalid before it would ever reach this call again (see
// that handler's own token-validation step).
func (db *DB) MarkPasswordResetTokenUsed(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE password_reset_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("store: mark password reset token %q used: %w", id, err)
	}
	return nil
}
