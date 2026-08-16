package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PasswordResetToken is one outstanding forgot-password reset attempt.
// TokenHash is the only form of the token ever persisted.
type PasswordResetToken struct {
	ID        string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ErrPasswordResetTokenNotFound is returned by
// GetPasswordResetTokenByHash when no row matches.
var ErrPasswordResetTokenNotFound = errors.New("store: password reset token not found")

// SavePasswordResetToken inserts a new reset-token row.
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
// regardless of expiry or used state: deciding whether the token is
// currently usable is the caller's job.
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
// permanently unusable for a second reset attempt.
func (db *DB) MarkPasswordResetTokenUsed(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE password_reset_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("store: mark password reset token %q used: %w", id, err)
	}
	return nil
}
