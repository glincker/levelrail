package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrSAMLRequestNotFound means no live AuthnRequest matched the browser.
	ErrSAMLRequestNotFound = errors.New("store: no pending saml request for this browser")
	// ErrSAMLAssertionReplayed means the assertion ID was already accepted once.
	ErrSAMLAssertionReplayed = errors.New("store: saml assertion already used")
	// ErrSAMLIdentityNotFound means no user is linked to the IDP subject.
	ErrSAMLIdentityNotFound = errors.New("store: saml identity not linked")
)

// SaveSAMLAuthnRequest records an outstanding AuthnRequest bound to the
// browser holding the secret behind browserHash, pruning expired ones.
func (db *DB) SaveSAMLAuthnRequest(ctx context.Context, id, connectionID, browserHash string, expires, now time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM saml_authn_requests WHERE expires_at <= ?`, now.Unix()); err != nil {
		return fmt.Errorf("store: prune saml requests: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO saml_authn_requests (id, connection_id, browser_hash, expires_at) VALUES (?, ?, ?, ?)`,
		id, connectionID, browserHash, expires.Unix())
	if err != nil {
		return fmt.Errorf("store: save saml request: %w", err)
	}
	return nil
}

// ConsumeSAMLAuthnRequest deletes and returns the newest live request for this
// connection and browser. It is single use: a second call finds nothing.
func (db *DB) ConsumeSAMLAuthnRequest(ctx context.Context, connectionID, browserHash string, now time.Time) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: consume saml request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM saml_authn_requests
		WHERE connection_id = ? AND browser_hash = ? AND expires_at > ? ORDER BY expires_at DESC LIMIT 1`,
		connectionID, browserHash, now.Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSAMLRequestNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: consume saml request: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM saml_authn_requests WHERE id = ?`, id)
	if err != nil {
		return "", fmt.Errorf("store: consume saml request: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", ErrSAMLRequestNotFound
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: consume saml request: %w", err)
	}
	return id, nil
}

// RecordSAMLAssertion remembers an accepted assertion ID until it expires, and
// fails with ErrSAMLAssertionReplayed if it was recorded before.
func (db *DB) RecordSAMLAssertion(ctx context.Context, connectionID, assertionID string, until, now time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM saml_seen_assertions WHERE expires_at <= ?`, now.Unix()); err != nil {
		return fmt.Errorf("store: prune saml assertions: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO saml_seen_assertions (connection_id, assertion_id, expires_at) VALUES (?, ?, ?)`,
		connectionID, assertionID, until.Unix())
	if isUniqueViolation(err) {
		return ErrSAMLAssertionReplayed
	}
	if err != nil {
		return fmt.Errorf("store: record saml assertion: %w", err)
	}
	return nil
}

// SAMLIdentityUser returns the user linked to an IDP subject.
func (db *DB) SAMLIdentityUser(ctx context.Context, connectionID, nameID string) (string, error) {
	var userID string
	err := db.QueryRowContext(ctx, `SELECT user_id FROM saml_identities WHERE connection_id = ? AND name_id = ?`,
		connectionID, nameID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSAMLIdentityNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: saml identity: %w", err)
	}
	return userID, nil
}

// LinkSAMLIdentity links an IDP subject to an existing user. A subject that is
// already linked to a different user is refused rather than moved.
func (db *DB) LinkSAMLIdentity(ctx context.Context, connectionID, nameID, userID string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO saml_identities (connection_id, name_id, user_id, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?)`, connectionID, nameID, userID, formatTime(now), formatTime(now))
	if isUniqueViolation(err) {
		existing, gerr := db.SAMLIdentityUser(ctx, connectionID, nameID)
		if gerr == nil && existing == userID {
			return nil
		}
		return ErrOAuthIdentityAlreadyLinked
	}
	if err != nil {
		return fmt.Errorf("store: link saml identity: %w", err)
	}
	return nil
}

// TouchSAMLIdentity stamps the last SAML sign-in of a linked subject.
func (db *DB) TouchSAMLIdentity(ctx context.Context, connectionID, nameID string, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE saml_identities SET last_login_at = ? WHERE connection_id = ? AND name_id = ?`,
		formatTime(now), connectionID, nameID); err != nil {
		return fmt.Errorf("store: touch saml identity: %w", err)
	}
	return nil
}

// ProvisionedUser is a user created by SSO or directory sync, with its role
// and identity link written in the same transaction.
type ProvisionedUser struct {
	User             User
	RoleID           string
	SAMLConnectionID string
	SAMLNameID       string
}

// CreateProvisionedUser inserts the user, assigns RoleID (copying its
// abilities), joins the default team and links the SAML subject atomically,
// so a failure never leaves an account without its role or link.
func (db *DB) CreateProvisionedUser(ctx context.Context, p ProvisionedUser) error {
	role, err := db.GetRole(ctx, p.RoleID)
	if err != nil {
		return err
	}
	abilities, err := json.Marshal(nonNilSlice(role.Abilities))
	if err != nil {
		return fmt.Errorf("store: provision user: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: provision user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	u := p.User
	_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, display_name, password_hash, abilities, is_first_user, created_at, last_login_at, role_id)
		VALUES (?, ?, ?, NULL, ?, 0, ?, NULL, ?)`, u.ID, u.Email, u.DisplayName, string(abilities), formatTime(u.CreatedAt), role.ID)
	if isUniqueViolation(err) && strings.Contains(err.Error(), "email") {
		return ErrUserEmailExists
	}
	if err != nil {
		return fmt.Errorf("store: provision user %q: %w", u.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		DefaultTeamID, u.ID, DefaultTeamRole(role.Abilities), formatTime(u.CreatedAt)); err != nil {
		return fmt.Errorf("store: provision user %q: join team: %w", u.ID, err)
	}
	if p.SAMLConnectionID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO saml_identities (connection_id, name_id, user_id, created_at, last_login_at)
			VALUES (?, ?, ?, ?, ?)`, p.SAMLConnectionID, p.SAMLNameID, u.ID, formatTime(u.CreatedAt), formatTime(u.CreatedAt)); err != nil {
			return fmt.Errorf("store: provision user %q: link identity: %w", u.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: provision user %q: %w", u.ID, err)
	}
	return nil
}
