package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// APIToken is one non-interactive credential (TASKS.md "Backend auth
// foundation"): a CLI, an MCP server, or a third-party integration
// authenticates with the plaintext token this row's TokenHash is a
// SHA-256 digest of, scoped to Abilities. The plaintext itself is never
// stored anywhere, generated once by internal/api and returned to the
// caller exactly once.
type APIToken struct {
	ID         string
	Name       string
	TokenHash  string
	Abilities  []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// ErrAPITokenNotFound is returned by GetAPITokenByHash and RevokeAPIToken
// when no token matches.
var ErrAPITokenNotFound = errors.New("store: api token not found")

// SaveAPIToken inserts a new token row. Tokens are never updated in
// place after creation except for LastUsedAt (TouchAPITokenLastUsed) and
// RevokedAt (RevokeAPIToken): a token's name and abilities are fixed at
// mint time, matching every competitor's own token model researched for
// this feature (rotation means minting a new token and revoking the
// old, not editing an existing one).
func (db *DB) SaveAPIToken(ctx context.Context, t APIToken) error {
	abilitiesJSON, err := json.Marshal(nonNilSlice(t.Abilities))
	if err != nil {
		return fmt.Errorf("store: marshal abilities for token %q: %w", t.ID, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, name, token_hash, abilities, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.TokenHash, string(abilitiesJSON), formatTime(t.CreatedAt), formatTimePtr(t.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: save api token %q: %w", t.ID, err)
	}
	return nil
}

// GetAPITokenByHash returns the token row matching hash, regardless of
// whether it's revoked or expired: deciding "is this token currently
// usable" is the caller's job (internal/api's auth middleware), this is
// pure lookup, the same separation reconcile_status.go keeps between
// storage and decision-making.
func (db *DB) GetAPITokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, token_hash, abilities, created_at, last_used_at, expires_at, revoked_at
		FROM api_tokens WHERE token_hash = ?
	`, hash)
	t, err := scanAPIToken(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAPITokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get api token by hash: %w", err)
	}
	return t, nil
}

// ListAPITokens returns every token, newest first, revoked ones
// included: the management UI shows revocation status rather than
// hiding history, matching Dokploy's own token list.
func (db *DB) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, token_hash, abilities, created_at, last_used_at, expires_at, revoked_at
		FROM api_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list api tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan api token row: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate api token rows: %w", err)
	}
	return out, nil
}

// RevokeAPIToken sets revoked_at on the named token. Idempotent: revoking
// an already-revoked token succeeds without error, matching
// DeleteDesiredService's "not found" sentinel shape for the one real
// failure case (no such token at all).
func (db *DB) RevokeAPIToken(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("store: revoke api token %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: revoke api token %q: rows affected: %w", id, err)
	}
	if n == 0 {
		// Either no such token, or it's already revoked (idempotent
		// success). Distinguish by checking existence, so a caller
		// revoking a truly nonexistent token still gets a 404-worthy
		// error rather than silent success.
		exists, err := db.apiTokenExists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrAPITokenNotFound
		}
	}
	return nil
}

func (db *DB) apiTokenExists(ctx context.Context, id string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM api_tokens WHERE id = ? LIMIT 1`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check api token %q exists: %w", id, err)
	}
	return true, nil
}

// TouchAPITokenLastUsed updates last_used_at to now. Best-effort by
// design: internal/api calls this on every successful bearer-token
// lookup for observability, but a failure here must never block the
// request it's authenticating, so callers log and continue rather than
// fail the request on error.
func (db *DB) TouchAPITokenLastUsed(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE api_tokens SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("store: touch api token %q last_used_at: %w", id, err)
	}
	return nil
}

func scanAPIToken(scan func(dest ...any) error) (*APIToken, error) {
	var (
		t                                APIToken
		abilitiesJSON                    string
		createdAt                        string
		lastUsedAt, expiresAt, revokedAt sql.NullString
	)
	if err := scan(&t.ID, &t.Name, &t.TokenHash, &abilitiesJSON, &createdAt, &lastUsedAt, &expiresAt, &revokedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(abilitiesJSON), &t.Abilities); err != nil {
		return nil, fmt.Errorf("unmarshal abilities: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	t.CreatedAt = parsed
	t.LastUsedAt, err = parseTimePtr(lastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_used_at: %w", err)
	}
	t.ExpiresAt, err = parseTimePtr(expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	t.RevokedAt, err = parseTimePtr(revokedAt)
	if err != nil {
		return nil, fmt.Errorf("parse revoked_at: %w", err)
	}
	return &t, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatTimePtr(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

func parseTimePtr(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
