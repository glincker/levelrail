package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserPreferences are one user's own display settings; empty strings mean "use the default".
type UserPreferences struct {
	UserID      string
	Timezone    string
	Language    string
	Theme       string
	LandingPage string
	Gravatar    bool
	UpdatedAt   time.Time
}

// GetUserPreferences returns the stored row, or an all-default value when the user never saved one.
func (db *DB) GetUserPreferences(ctx context.Context, userID string) (UserPreferences, error) {
	p := UserPreferences{UserID: userID}
	var updated string
	var gravatar int
	err := db.QueryRowContext(ctx, `
		SELECT timezone, language, theme, landing_page, gravatar, updated_at
		FROM user_preferences WHERE user_id = ?`, userID).
		Scan(&p.Timezone, &p.Language, &p.Theme, &p.LandingPage, &gravatar, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("store: get preferences for %q: %w", userID, err)
	}
	p.Gravatar = gravatar != 0
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return p, nil
}

// SetUserPreferences replaces the user's preferences row.
func (db *DB) SetUserPreferences(ctx context.Context, p UserPreferences) error {
	gravatar := 0
	if p.Gravatar {
		gravatar = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, timezone, language, theme, landing_page, gravatar, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET timezone = excluded.timezone, language = excluded.language,
			theme = excluded.theme, landing_page = excluded.landing_page, gravatar = excluded.gravatar,
			updated_at = excluded.updated_at`,
		p.UserID, p.Timezone, p.Language, p.Theme, p.LandingPage, gravatar, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set preferences for %q: %w", p.UserID, err)
	}
	return nil
}

// DeleteOAuthIdentityForUser unlinks one provider from one user.
func (db *DB) DeleteOAuthIdentityForUser(ctx context.Context, userID, provider string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM user_oauth_identities WHERE user_id = ? AND provider = ?`, userID, provider)
	if err != nil {
		return fmt.Errorf("store: unlink %q from user %q: %w", provider, userID, err)
	}
	return rowsAffectedOrNotFound(res, ErrOAuthIdentityNotFound, "unlink identity of user %q", userID)
}

// EmailChangeRequest is one pending, verify-by-link email change.
type EmailChangeRequest struct {
	ID        string
	UserID    string
	NewEmail  string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ErrEmailChangeInvalid covers an unknown, expired, used or foreign verification token.
var ErrEmailChangeInvalid = errors.New("store: email change request invalid")

// CreateEmailChangeRequest stores a pending change and drops the user's older unused ones.
func (db *DB) CreateEmailChangeRequest(ctx context.Context, req EmailChangeRequest) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin email change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_change_requests WHERE user_id = ? AND used_at IS NULL`, req.UserID); err != nil {
		return fmt.Errorf("store: clear old email changes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO email_change_requests (id, user_id, new_email, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		req.ID, req.UserID, req.NewEmail, req.TokenHash,
		req.CreatedAt.UTC().Format(time.RFC3339Nano), req.ExpiresAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: create email change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit email change: %w", err)
	}
	return nil
}

// ConsumeEmailChange marks the token used and returns the new address; the token must belong to userID.
func (db *DB) ConsumeEmailChange(ctx context.Context, tokenHash, userID string, now time.Time) (string, error) {
	var newEmail string
	err := db.QueryRowContext(ctx, `
		UPDATE email_change_requests SET used_at = ?
		WHERE token_hash = ? AND user_id = ? AND used_at IS NULL AND expires_at > ?
		RETURNING new_email`,
		now.UTC().Format(time.RFC3339Nano), tokenHash, userID, now.UTC().Format(time.RFC3339Nano)).Scan(&newEmail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrEmailChangeInvalid
	}
	if err != nil {
		return "", fmt.Errorf("store: consume email change: %w", err)
	}
	return newEmail, nil
}

// UpdateUserEmail sets a user's email, ErrUserEmailExists when another account holds it.
func (db *DB) UpdateUserEmail(ctx context.Context, id, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	var taken string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ? AND id != ?`, email, id).Scan(&taken)
	if err == nil {
		return ErrUserEmailExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: check email %q: %w", email, err)
	}
	res, err := db.ExecContext(ctx, `UPDATE users SET email = ? WHERE id = ?`, email, id)
	if err != nil {
		return fmt.Errorf("store: update user %q email: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrUserNotFound, "update user %q email", id)
}
