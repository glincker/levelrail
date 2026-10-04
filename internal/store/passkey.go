package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PasskeyCredential is one WebAuthn credential linked to a user
// (migrations/0255). PublicKey is COSE-encoded public key material, not
// a secret, so unlike a TOTP secret it's an ordinary column rather than
// something routed through internal/secrets. CredentialID is the
// authenticator's own credential ID, base64url-encoded the same way
// every other opaque token in this schema is stored as text.
type PasskeyCredential struct {
	ID           string
	UserID       string
	CredentialID string
	PublicKey    []byte
	SignCount    uint32
	AAGUID       string
	Transports   []string
	Label        string
	// BackupEligible and BackupState are the authenticator's BE/BS flags
	// as recorded at registration (migrations/0280). FinishLogin compares
	// BackupEligible against the live assertion on every sign-in and
	// rejects a mismatch, so this must be the real registration-time
	// value, never a zero-value default.
	BackupEligible bool
	BackupState    bool
	CreatedAt      time.Time
	LastUsedAt     *time.Time
}

// ErrPasskeyCredentialNotFound is returned by DeletePasskeyCredential
// when no row matches (id, userID).
var ErrPasskeyCredentialNotFound = errors.New("store: passkey credential not found")

// ErrPasskeyCredentialAlreadyRegistered is SavePasskeyCredential's
// failure mode when the authenticator's credential ID is already linked
// to some account, own or otherwise: the unique index
// (migrations/0255) enforces this at the database level.
var ErrPasskeyCredentialAlreadyRegistered = errors.New("store: passkey credential already registered")

// SavePasskeyCredential inserts a newly registered credential, insert-only.
func (db *DB) SavePasskeyCredential(ctx context.Context, c PasskeyCredential) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_passkeys (id, user_id, credential_id, public_key, sign_count, aaguid, transports, label, backup_eligible, backup_state, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, c.ID, c.UserID, c.CredentialID, c.PublicKey, c.SignCount, c.AAGUID, strings.Join(c.Transports, ","), c.Label, c.BackupEligible, c.BackupState, formatTime(c.CreatedAt))
	if err == nil {
		return nil
	}
	if _, getErr := db.getPasskeyCredentialByCredentialID(ctx, c.CredentialID); getErr == nil {
		return ErrPasskeyCredentialAlreadyRegistered
	}
	return fmt.Errorf("store: save passkey credential: %w", err)
}

func (db *DB) getPasskeyCredentialByCredentialID(ctx context.Context, credentialID string) (*PasskeyCredential, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, user_id, credential_id, public_key, sign_count, aaguid, transports, label, backup_eligible, backup_state, created_at, last_used_at
		FROM user_passkeys WHERE credential_id = ?
	`, credentialID)
	return scanPasskeyCredential(row.Scan)
}

// ListPasskeyCredentialsForUser returns every credential linked to
// userID, oldest first: both the settings-page listing and the login
// ceremony's own "which credentials does this account hold" lookup.
func (db *DB) ListPasskeyCredentialsForUser(ctx context.Context, userID string) ([]PasskeyCredential, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, credential_id, public_key, sign_count, aaguid, transports, label, backup_eligible, backup_state, created_at, last_used_at
		FROM user_passkeys WHERE user_id = ? ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list passkey credentials for user %q: %w", userID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PasskeyCredential
	for rows.Next() {
		c, err := scanPasskeyCredential(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan passkey credential row: %w", err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate passkey credential rows: %w", err)
	}
	return out, nil
}

// UpdatePasskeySignCountByCredentialID bumps a credential's stored sign
// counter and last_used_at after a successful login ceremony, keyed by
// the authenticator's own credential ID rather than the row ID: that's
// what a login response identifies, the row ID is never sent back by
// the client.
func (db *DB) UpdatePasskeySignCountByCredentialID(ctx context.Context, credentialID string, signCount uint32, usedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE user_passkeys SET sign_count = ?, last_used_at = ? WHERE credential_id = ?
	`, signCount, formatTime(usedAt), credentialID)
	if err != nil {
		return fmt.Errorf("store: update passkey sign count for credential %q: %w", credentialID, err)
	}
	return rowsAffectedOrNotFound(res, ErrPasskeyCredentialNotFound, "update passkey sign count for credential %q", credentialID)
}

// DeletePasskeyCredential removes credential id, scoped to userID so an
// account can only ever revoke its own credentials.
func (db *DB) DeletePasskeyCredential(ctx context.Context, id, userID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM user_passkeys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("store: delete passkey credential %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrPasskeyCredentialNotFound, "delete passkey credential %q", id)
}

func scanPasskeyCredential(scan func(dest ...any) error) (*PasskeyCredential, error) {
	var (
		c          PasskeyCredential
		transports string
		createdAt  string
		lastUsedAt sql.NullString
	)
	if err := scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.SignCount, &c.AAGUID, &transports, &c.Label, &c.BackupEligible, &c.BackupState, &createdAt, &lastUsedAt); err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	c.CreatedAt = parsed
	last, err := parseTimePtr(lastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_used_at: %w", err)
	}
	c.LastUsedAt = last
	if transports != "" {
		c.Transports = strings.Split(transports, ",")
	}
	return &c, nil
}
