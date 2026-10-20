package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// ErrOwnerNotFound means the legacy user does not exist in the users table.
var ErrOwnerNotFound = errors.New("authengine: token owner not found")

// EngineUserID returns the library id mapped to a legacy user id.
func (d *Directory) EngineUserID(ctx context.Context, legacyID string) (string, bool, error) {
	return d.lookup(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacyID)
}

// LegacyUserID returns the legacy user id mapped to a library id.
func (d *Directory) LegacyUserID(ctx context.Context, engineID string) (string, bool, error) {
	return d.lookup(ctx, `SELECT legacy_id FROM authengine_user_map WHERE engine_id = ?`, engineID)
}

// EngineTokenID returns the library token id mapped to a legacy token id.
func (d *Directory) EngineTokenID(ctx context.Context, legacyID string) (string, bool, error) {
	return d.lookup(ctx, `SELECT engine_id FROM authengine_token_map WHERE legacy_id = ?`, legacyID)
}

// LegacyTokenID returns the legacy token id mapped to a library token id.
func (d *Directory) LegacyTokenID(ctx context.Context, engineID string) (string, bool, error) {
	return d.lookup(ctx, `SELECT legacy_id FROM authengine_token_map WHERE engine_id = ?`, engineID)
}

// LinkToken records that a legacy token row and a library token row are the same token.
func (d *Directory) LinkToken(ctx context.Context, legacyID, engineID string) error {
	_, err := d.db.ExecContext(ctx, `INSERT OR IGNORE INTO authengine_token_map (legacy_id, engine_id) VALUES (?, ?)`, legacyID, engineID)
	if err != nil {
		return fmt.Errorf("authengine: link token %s: %w", legacyID, err)
	}
	return nil
}

func (d *Directory) lookup(ctx context.Context, query, arg string) (string, bool, error) {
	var out string
	err := d.db.QueryRowContext(ctx, query, arg).Scan(&out)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("authengine: id map lookup: %w", err)
	}
	return out, true, nil
}

// EnsureEngineUser maps a legacy user into the library, creating the minimal
// library row when the backfill has not seen the user yet. Idempotent.
func (d *Directory) EnsureEngineUser(ctx context.Context, legacyID string) (string, error) {
	if id, ok, err := d.EngineUserID(ctx, legacyID); err != nil || ok {
		return id, err
	}
	var email, name, created string
	err := d.db.QueryRowContext(ctx, `SELECT email, display_name, created_at FROM users WHERE id = ?`, legacyID).Scan(&email, &name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrOwnerNotFound
	}
	if err != nil {
		return "", fmt.Errorf("authengine: load user %s: %w", legacyID, err)
	}
	at, perr := time.Parse(time.RFC3339Nano, created)
	if perr != nil {
		at = time.Now()
	}
	engineID := ulid.Make().String()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("authengine: begin user map: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_user_map (legacy_id, engine_id) VALUES (?, ?)`, legacyID, engineID); err != nil {
		return "", fmt.Errorf("authengine: map user %s: %w", legacyID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO theauth_users (id, email, name, display_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
		engineID, strings.ToLower(email), name, name, at.UTC().UnixMicro(), at.UTC().UnixMicro()); err != nil {
		return "", fmt.Errorf("authengine: create library user for %s: %w", legacyID, err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("authengine: commit user map: %w", err)
	}
	return engineID, nil
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}
