package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"
)

func parseULID(s string) (theauth.ULID, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return theauth.ULID{}, fmt.Errorf("authengine: parse ulid: %w", err)
	}
	return id, nil
}

// LegacyUser is the slice of a platform user the library needs.
type LegacyUser struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	CreatedAt    time.Time
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// SyncUser makes the library user for u exist and match: it creates the
// id-map row and library user when missing, and mirrors the password hash
// (bcrypt is accepted by the library and rehashed on the next login). An
// unmapped library user holding the same email is an orphan and is replaced.
func (s *Sessions) SyncUser(ctx context.Context, u LegacyUser) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("authengine: begin user sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var engineID string
	err = tx.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, u.ID).Scan(&engineID)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		engineID = ulid.Make().String()
		if err := dropOrphan(ctx, tx, normEmail(u.Email)); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_user_map (legacy_id, engine_id) VALUES (?, ?)`, u.ID, engineID); err != nil {
			return "", fmt.Errorf("authengine: map user %s: %w", u.ID, err)
		}
	default:
		return "", fmt.Errorf("authengine: look up user map for %s: %w", u.ID, err)
	}

	created := u.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	now := time.Now().UTC().UnixMicro()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO theauth_users (id, email, name, display_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET email = excluded.email, name = excluded.name,
			display_name = excluded.display_name, updated_at = excluded.updated_at`,
		engineID, normEmail(u.Email), u.DisplayName, u.DisplayName, created.UTC().UnixMicro(), now); err != nil {
		return "", fmt.Errorf("authengine: sync library user %s: %w", u.ID, err)
	}
	if err := upsertPassword(ctx, tx, engineID, u.PasswordHash); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("authengine: commit user sync: %w", err)
	}
	return engineID, nil
}

func dropOrphan(ctx context.Context, tx *sql.Tx, email string) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM theauth_users WHERE email = ?
		AND id NOT IN (SELECT engine_id FROM authengine_user_map)`, email); err != nil {
		return fmt.Errorf("authengine: drop orphan library user: %w", err)
	}
	return nil
}

func upsertPassword(ctx context.Context, tx *sql.Tx, engineID, hash string) error {
	if hash == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO theauth_user_passwords (user_id, password_hash) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET password_hash = excluded.password_hash`, engineID, hash); err != nil {
		return fmt.Errorf("authengine: sync password hash: %w", err)
	}
	return nil
}

// SyncPassword mirrors a new platform password hash into the library and
// reports the library id so the caller can audit the change.
func (s *Sessions) SyncPassword(ctx context.Context, legacyUserID, hash string) (string, error) {
	engine, err := s.engineID(ctx, legacyUserID)
	if err != nil || engine == "" {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("authengine: begin password sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if hash == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM theauth_user_passwords WHERE user_id = ?`, engine); err != nil {
			return "", fmt.Errorf("authengine: clear password hash: %w", err)
		}
	} else if err := upsertPassword(ctx, tx, engine, hash); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("authengine: commit password sync: %w", err)
	}
	return engine, nil
}

// DeleteUser removes a user's library side: sessions, tokens and the user
// itself. Call it after the platform user is deleted: the id-map cascade has
// already made every library session of that user unresolvable.
func (s *Sessions) DeleteUser(ctx context.Context, engineID string) error {
	if engineID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("authengine: begin user delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().UnixMicro()
	stmts := []struct {
		q    string
		args []any
	}{
		{`UPDATE theauth_api_tokens SET revoked_at = ? WHERE owner_id = ? AND revoked_at IS NULL`, []any{now, engineID}},
		{`DELETE FROM theauth_sessions WHERE user_id = ?`, []any{engineID}},
		{`DELETE FROM theauth_session_links WHERE user_id = ?`, []any{engineID}},
		{`DELETE FROM theauth_users WHERE id = ?`, []any{engineID}},
	}
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.q, st.args...); err != nil {
			return fmt.Errorf("authengine: delete library user %s: %w", engineID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("authengine: commit user delete: %w", err)
	}
	return nil
}

// EngineIDFor returns the library id mapped to a platform user, or "".
func (s *Sessions) EngineIDFor(ctx context.Context, legacyUserID string) (string, error) {
	return s.engineID(ctx, legacyUserID)
}

// LinkUser maps a platform user to an existing library user (first-run registration).
func (s *Sessions) LinkUser(ctx context.Context, legacyUserID, engineID string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO authengine_user_map (legacy_id, engine_id) VALUES (?, ?)`, legacyUserID, engineID); err != nil {
		return fmt.Errorf("authengine: link user %s: %w", legacyUserID, err)
	}
	return nil
}

// ensureSyncedByEmail creates the library user for a platform user that has
// none yet (created before the switch, or by a path that skipped the hook).
func (s *Sessions) ensureSyncedByEmail(ctx context.Context, email string) error {
	var u LegacyUser
	var hash sql.NullString
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.display_name, u.password_hash, u.created_at FROM users u
		LEFT JOIN authengine_user_map m ON m.legacy_id = u.id
		WHERE lower(u.email) = lower(?) AND m.legacy_id IS NULL`, strings.TrimSpace(email)).Scan(&u.ID, &u.Email, &u.DisplayName, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("authengine: look up unsynced user: %w", err)
	}
	u.PasswordHash = hash.String
	if t, perr := parseLegacyTime(created); perr == nil {
		u.CreatedAt = t
	}
	if _, err := s.SyncUser(ctx, u); err != nil {
		return err
	}
	return nil
}

func parseLegacyTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("authengine: parse timestamp: %w", err)
	}
	return t, nil
}
