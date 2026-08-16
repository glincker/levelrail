package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OAuth provider names the migrations/0035_users.sql CHECK constraints
// accept. Fixed pair, no other providers (Phase 4 scope).
const (
	OAuthProviderGoogle = "google"
	OAuthProviderGitHub = "github"
)

// OAuthIdentity is one linked external account. Uniqueness (at most one
// per provider per user, never linked to two users) is enforced by
// migrations/0035's own indexes, not application code.
type OAuthIdentity struct {
	ID             string
	UserID         string
	Provider       string
	ProviderUserID string
	CreatedAt      time.Time
}

// ErrOAuthIdentityNotFound is returned by GetOAuthIdentity when no row
// matches.
var ErrOAuthIdentityNotFound = errors.New("store: oauth identity not found")

// ErrOAuthIdentityAlreadyLinked is SaveOAuthIdentity's failure mode when
// either unique index (migrations/0035) would be violated.
var ErrOAuthIdentityAlreadyLinked = errors.New("store: oauth identity already linked")

// SaveOAuthIdentity inserts a new linked identity, insert-only, never an
// upsert.
func (db *DB) SaveOAuthIdentity(ctx context.Context, i OAuthIdentity) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_oauth_identities (id, user_id, provider, provider_user_id, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, i.ID, i.UserID, i.Provider, i.ProviderUserID, formatTime(i.CreatedAt))
	if err == nil {
		return nil
	}

	// Re-check by the two things that could have raced, rather than
	// assuming every INSERT failure means the same thing.
	if _, getErr := db.GetOAuthIdentity(ctx, i.Provider, i.ProviderUserID); getErr == nil {
		return ErrOAuthIdentityAlreadyLinked
	}
	existing, listErr := db.ListOAuthIdentitiesForUser(ctx, i.UserID)
	if listErr == nil {
		for _, e := range existing {
			if e.Provider == i.Provider {
				return ErrOAuthIdentityAlreadyLinked
			}
		}
	}
	return fmt.Errorf("store: save oauth identity: %w", err)
}

// GetOAuthIdentity returns the identity linking (provider, providerUserID)
// to whichever user holds it, or ErrOAuthIdentityNotFound. This is the
// OAuth callback's primary lookup: "has this external account signed in
// here before."
func (db *DB) GetOAuthIdentity(ctx context.Context, provider, providerUserID string) (*OAuthIdentity, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, provider_user_id, created_at
		FROM user_oauth_identities WHERE provider = ? AND provider_user_id = ?
	`, provider, providerUserID)
	i, err := scanOAuthIdentity(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get oauth identity: %w", err)
	}
	return i, nil
}

// ListOAuthIdentitiesForUser returns every provider a user has linked,
// oldest first.
func (db *DB) ListOAuthIdentitiesForUser(ctx context.Context, userID string) ([]OAuthIdentity, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, provider, provider_user_id, created_at
		FROM user_oauth_identities WHERE user_id = ? ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list oauth identities for user %q: %w", userID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []OAuthIdentity
	for rows.Next() {
		i, err := scanOAuthIdentity(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan oauth identity row: %w", err)
		}
		out = append(out, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate oauth identity rows: %w", err)
	}
	return out, nil
}

func scanOAuthIdentity(scan func(dest ...any) error) (*OAuthIdentity, error) {
	var (
		i         OAuthIdentity
		createdAt string
	)
	if err := scan(&i.ID, &i.UserID, &i.Provider, &i.ProviderUserID, &createdAt); err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	i.CreatedAt = parsed
	return &i, nil
}
