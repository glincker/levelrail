package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrSCIMTokenNotFound is returned for an unknown or already revoked token ID.
var ErrSCIMTokenNotFound = errors.New("store: scim token not found")

// SCIMToken is one directory sync bearer token; only its SHA-256 is stored.
type SCIMToken struct {
	ID         string
	Name       string
	TokenHash  string
	Prefix     string
	CreatedBy  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

const scimTokenColumns = `id, name, token_hash, prefix, created_by, created_at, last_used_at, revoked_at` //nolint:gosec // column list, not a credential

func scanSCIMToken(scan func(dest ...any) error) (SCIMToken, error) {
	var (
		t             SCIMToken
		created       string
		used, revoked sql.NullString
	)
	err := scan(&t.ID, &t.Name, &t.TokenHash, &t.Prefix, &t.CreatedBy, &created, &used, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return SCIMToken{}, ErrSCIMTokenNotFound
	}
	if err != nil {
		return SCIMToken{}, fmt.Errorf("store: scan scim token: %w", err)
	}
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return SCIMToken{}, fmt.Errorf("store: scim token created_at: %w", err)
	}
	if t.LastUsedAt, err = parseTimePtr(used); err != nil {
		return SCIMToken{}, fmt.Errorf("store: scim token last_used_at: %w", err)
	}
	if t.RevokedAt, err = parseTimePtr(revoked); err != nil {
		return SCIMToken{}, fmt.Errorf("store: scim token revoked_at: %w", err)
	}
	return t, nil
}

// CreateSCIMToken stores a new token.
func (db *DB) CreateSCIMToken(ctx context.Context, t SCIMToken) error {
	_, err := db.ExecContext(ctx, `INSERT INTO scim_tokens (id, name, token_hash, prefix, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.TokenHash, t.Prefix, t.CreatedBy, formatTime(t.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: create scim token: %w", err)
	}
	return nil
}

// ListSCIMTokens returns every token, newest first, revoked ones included.
func (db *DB) ListSCIMTokens(ctx context.Context) ([]SCIMToken, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+scimTokenColumns+` FROM scim_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list scim tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SCIMToken
	for rows.Next() {
		t, err := scanSCIMToken(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list scim tokens: %w", err)
	}
	return out, nil
}

// ActiveSCIMTokenByHash returns the unrevoked token with this hash.
func (db *DB) ActiveSCIMTokenByHash(ctx context.Context, hash string) (SCIMToken, error) {
	return scanSCIMToken(db.QueryRowContext(ctx, `SELECT `+scimTokenColumns+` FROM scim_tokens WHERE token_hash = ? AND revoked_at IS NULL`, hash).Scan)
}

// RevokeSCIMToken revokes an active token.
func (db *DB) RevokeSCIMToken(ctx context.Context, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE scim_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("store: revoke scim token %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrSCIMTokenNotFound, "revoke scim token %q", id)
}

// RotateSCIMToken revokes oldID and stores next in one transaction, so a
// rotation never leaves both tokens working or neither.
func (db *DB) RotateSCIMToken(ctx context.Context, oldID string, next SCIMToken) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: rotate scim token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE scim_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(next.CreatedAt), oldID)
	if err != nil {
		return fmt.Errorf("store: rotate scim token: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrSCIMTokenNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scim_tokens (id, name, token_hash, prefix, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		next.ID, next.Name, next.TokenHash, next.Prefix, next.CreatedBy, formatTime(next.CreatedAt)); err != nil {
		return fmt.Errorf("store: rotate scim token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: rotate scim token: %w", err)
	}
	return nil
}

// GetSCIMToken returns one token by ID.
func (db *DB) GetSCIMToken(ctx context.Context, id string) (SCIMToken, error) {
	return scanSCIMToken(db.QueryRowContext(ctx, `SELECT `+scimTokenColumns+` FROM scim_tokens WHERE id = ?`, id).Scan)
}

// TouchSCIMToken stamps last use.
func (db *DB) TouchSCIMToken(ctx context.Context, id string, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE scim_tokens SET last_used_at = ? WHERE id = ?`, formatTime(now), id); err != nil {
		return fmt.Errorf("store: touch scim token: %w", err)
	}
	return nil
}
