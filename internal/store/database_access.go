package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Database access user kinds and states.
const (
	DatabaseAccessKindUser = "user"
	DatabaseAccessKindTemp = "temp"

	DatabaseAccessStateActive   = "active"
	DatabaseAccessStateRevoking = "revoking"
	DatabaseAccessStateRevoked  = "revoked"

	// DatabaseNetworkScopePlatform is the default scope.
	DatabaseNetworkScopePlatform = "platform"
)

// DatabaseAccessTimeLayout is the fixed-width UTC layout used for every
// timestamp here, so lexical comparison in SQL is chronological.
const DatabaseAccessTimeLayout = "2006-01-02T15:04:05Z"

// DatabaseAccessUser tracks a role this platform created in a managed
// database. It never holds a password.
type DatabaseAccessUser struct {
	ID        string
	Database  string
	Role      string
	Kind      string
	Preset    string
	CreatedBy string
	CreatedAt string
	ExpiresAt string
	State     string
	RevokedAt string
}

// ErrDatabaseAccessUserExists is returned when a live record already holds the role.
var ErrDatabaseAccessUserExists = errors.New("store: database access user already exists")

// DatabaseAccessSettings is a database's network scope and TLS policy.
type DatabaseAccessSettings struct {
	Database   string
	Scope      string
	RequireTLS bool
	UpdatedAt  string
}

// RecordDatabaseAccessUser inserts a live access user.
func (db *DB) RecordDatabaseAccessUser(ctx context.Context, u DatabaseAccessUser) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_access_users (id, database_name, role, kind, preset, created_by, created_at, expires_at, state)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, u.ID, u.Database, u.Role, u.Kind, u.Preset, u.CreatedBy, u.CreatedAt, u.ExpiresAt, DatabaseAccessStateActive)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDatabaseAccessUserExists
		}
		return fmt.Errorf("store: record database access user %q on %q: %w", u.Role, u.Database, err)
	}
	return nil
}

const databaseAccessUserColumns = `id, database_name, role, kind, preset, created_by, created_at, COALESCE(expires_at, ''), state, COALESCE(revoked_at, '')`

func scanDatabaseAccessUser(scan func(...any) error) (DatabaseAccessUser, error) {
	var u DatabaseAccessUser
	err := scan(&u.ID, &u.Database, &u.Role, &u.Kind, &u.Preset, &u.CreatedBy, &u.CreatedAt, &u.ExpiresAt, &u.State, &u.RevokedAt)
	return u, err
}

// ListDatabaseAccessUsers returns the live users of one kind for a database,
// oldest first. An empty kind returns every kind.
func (db *DB) ListDatabaseAccessUsers(ctx context.Context, database, kind string) ([]DatabaseAccessUser, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+databaseAccessUserColumns+` FROM database_access_users
		WHERE database_name = ? AND state != ? AND (? = '' OR kind = ?)
		ORDER BY created_at
	`, database, DatabaseAccessStateRevoked, kind, kind)
	if err != nil {
		return nil, fmt.Errorf("store: list database access users for %q: %w", database, err)
	}
	return collectDatabaseAccessUsers(rows)
}

func collectDatabaseAccessUsers(rows *sql.Rows) ([]DatabaseAccessUser, error) {
	defer func() { _ = rows.Close() }()
	var out []DatabaseAccessUser
	for rows.Next() {
		u, err := scanDatabaseAccessUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan database access user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate database access users: %w", err)
	}
	return out, nil
}

// GetDatabaseAccessUser returns the live record for a role, or nil.
func (db *DB) GetDatabaseAccessUser(ctx context.Context, database, role string) (*DatabaseAccessUser, error) {
	u, err := scanDatabaseAccessUser(db.QueryRowContext(ctx, `
		SELECT `+databaseAccessUserColumns+` FROM database_access_users
		WHERE database_name = ? AND role = ? AND state != ?
	`, database, role, DatabaseAccessStateRevoked).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get database access user %q on %q: %w", role, database, err)
	}
	return &u, nil
}

// GetDatabaseAccessUserByID returns a record by id regardless of state, or nil.
func (db *DB) GetDatabaseAccessUserByID(ctx context.Context, id string) (*DatabaseAccessUser, error) {
	u, err := scanDatabaseAccessUser(db.QueryRowContext(ctx, `
		SELECT `+databaseAccessUserColumns+` FROM database_access_users WHERE id = ?
	`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get database access user %q: %w", id, err)
	}
	return &u, nil
}

// MarkDatabaseAccessUserRevoked retires a live record for a role.
func (db *DB) MarkDatabaseAccessUserRevoked(ctx context.Context, database, role string, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE database_access_users SET state = ?, revoked_at = ?
		WHERE database_name = ? AND role = ? AND state != ?
	`, DatabaseAccessStateRevoked, now.UTC().Format(DatabaseAccessTimeLayout), database, role, DatabaseAccessStateRevoked)
	if err != nil {
		return fmt.Errorf("store: revoke database access user %q on %q: %w", role, database, err)
	}
	return nil
}

// DueDatabaseAccessUsers lists temporary credentials whose expiry has passed
// and that are not yet revoked.
func (db *DB) DueDatabaseAccessUsers(ctx context.Context, now time.Time) ([]DatabaseAccessUser, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+databaseAccessUserColumns+` FROM database_access_users
		WHERE kind = ? AND state != ? AND expires_at IS NOT NULL AND expires_at <= ?
		ORDER BY expires_at
	`, DatabaseAccessKindTemp, DatabaseAccessStateRevoked, now.UTC().Format(DatabaseAccessTimeLayout))
	if err != nil {
		return nil, fmt.Errorf("store: list due database access users: %w", err)
	}
	return collectDatabaseAccessUsers(rows)
}

// ClaimDatabaseAccessUser atomically moves an active record to revoking. A
// revoking record is retaken once its claim is older than lease.
func (db *DB) ClaimDatabaseAccessUser(ctx context.Context, id string, now time.Time, lease time.Duration) (bool, error) {
	n := now.UTC().Format(DatabaseAccessTimeLayout)
	stale := now.Add(-lease).UTC().Format(DatabaseAccessTimeLayout)
	res, err := db.ExecContext(ctx, `
		UPDATE database_access_users SET state = ?, claimed_at = ?
		WHERE id = ? AND (state = ? OR (state = ? AND (claimed_at IS NULL OR claimed_at <= ?)))
	`, DatabaseAccessStateRevoking, n, id, DatabaseAccessStateActive, DatabaseAccessStateRevoking, stale)
	if err != nil {
		return false, fmt.Errorf("store: claim database access user %q: %w", id, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: claim database access user %q: rows affected: %w", id, err)
	}
	return rows == 1, nil
}

// CompleteDatabaseAccessUser marks a claimed record revoked.
func (db *DB) CompleteDatabaseAccessUser(ctx context.Context, id string, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE database_access_users SET state = ?, revoked_at = ? WHERE id = ?
	`, DatabaseAccessStateRevoked, now.UTC().Format(DatabaseAccessTimeLayout), id)
	if err != nil {
		return fmt.Errorf("store: complete database access user %q: %w", id, err)
	}
	return nil
}

// ReleaseDatabaseAccessUser returns a claimed record to active for the next pass.
func (db *DB) ReleaseDatabaseAccessUser(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE database_access_users SET state = ?, claimed_at = NULL WHERE id = ? AND state = ?
	`, DatabaseAccessStateActive, id, DatabaseAccessStateRevoking)
	if err != nil {
		return fmt.Errorf("store: release database access user %q: %w", id, err)
	}
	return nil
}

// GetDatabaseAccessSettings returns a database's settings; a database with
// none has the platform-wide scope and optional TLS.
func (db *DB) GetDatabaseAccessSettings(ctx context.Context, database string) (DatabaseAccessSettings, error) {
	s := DatabaseAccessSettings{Database: database, Scope: DatabaseNetworkScopePlatform}
	var tls int
	err := db.QueryRowContext(ctx, `
		SELECT network_scope, require_tls, updated_at FROM database_access_settings WHERE database_name = ?
	`, database).Scan(&s.Scope, &tls, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("store: get database access settings for %q: %w", database, err)
	}
	s.RequireTLS = tls == 1
	return s, nil
}

// SaveDatabaseAccessSettings inserts or replaces a database's settings.
func (db *DB) SaveDatabaseAccessSettings(ctx context.Context, s DatabaseAccessSettings) error {
	tls := 0
	if s.RequireTLS {
		tls = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_access_settings (database_name, network_scope, require_tls, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (database_name) DO UPDATE SET
			network_scope = excluded.network_scope, require_tls = excluded.require_tls, updated_at = excluded.updated_at
	`, s.Database, s.Scope, tls, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save database access settings for %q: %w", s.Database, err)
	}
	return nil
}

// ListDatabaseNetworkRuleNotes returns source -> description for a database.
func (db *DB) ListDatabaseNetworkRuleNotes(ctx context.Context, database string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT source, description FROM database_network_rule_notes WHERE database_name = ?
	`, database)
	if err != nil {
		return nil, fmt.Errorf("store: list network rule notes for %q: %w", database, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var src, desc string
		if err := rows.Scan(&src, &desc); err != nil {
			return nil, fmt.Errorf("store: scan network rule note: %w", err)
		}
		out[src] = desc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate network rule notes: %w", err)
	}
	return out, nil
}

// ReplaceDatabaseNetworkRuleNotes swaps a database's notes for notes.
func (db *DB) ReplaceDatabaseNetworkRuleNotes(ctx context.Context, database string, notes map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin network rule notes for %q: %w", database, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_network_rule_notes WHERE database_name = ?`, database); err != nil {
		return fmt.Errorf("store: clear network rule notes for %q: %w", database, err)
	}
	for src, desc := range notes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO database_network_rule_notes (database_name, source, description) VALUES (?, ?, ?)
		`, database, src, desc); err != nil {
			return fmt.Errorf("store: save network rule note for %q: %w", database, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit network rule notes for %q: %w", database, err)
	}
	return nil
}

// NewDatabaseAccessID returns an opaque identifier for an access user record.
func NewDatabaseAccessID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: generate database access id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
