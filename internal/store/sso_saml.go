package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrSAMLConnectionNotFound is returned when no connection has the given ID.
	ErrSAMLConnectionNotFound = errors.New("store: saml connection not found")
	// ErrSAMLConnectionNameTaken is returned when a connection name is already used.
	ErrSAMLConnectionNameTaken = errors.New("store: a saml connection with that name already exists")
)

// SAMLConnection is one trusted SAML identity provider (migration 0610).
type SAMLConnection struct {
	ID              string
	Name            string
	Enabled         bool
	IDPEntityID     string
	IDPSSOURL       string
	IDPCertificates string
	IDPMetadataURL  string
	EmailAttribute  string
	NameAttribute   string
	GroupsAttribute string
	AllowedDomains  []string
	JITProvisioning bool
	DefaultRoleID   string
	SyncRoles       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const samlConnectionColumns = `id, name, enabled, idp_entity_id, idp_sso_url, idp_certificates, idp_metadata_url,
	email_attribute, name_attribute, groups_attribute, allowed_domains, jit_provisioning, default_role_id,
	sync_roles, created_at, updated_at`

func scanSAMLConnection(scan func(dest ...any) error) (SAMLConnection, error) {
	var (
		c                       SAMLConnection
		enabled, jit, sync      int
		domains, created, updat string
	)
	err := scan(&c.ID, &c.Name, &enabled, &c.IDPEntityID, &c.IDPSSOURL, &c.IDPCertificates, &c.IDPMetadataURL,
		&c.EmailAttribute, &c.NameAttribute, &c.GroupsAttribute, &domains, &jit, &c.DefaultRoleID, &sync, &created, &updat)
	if errors.Is(err, sql.ErrNoRows) {
		return SAMLConnection{}, ErrSAMLConnectionNotFound
	}
	if err != nil {
		return SAMLConnection{}, fmt.Errorf("store: scan saml connection: %w", err)
	}
	if err := json.Unmarshal([]byte(domains), &c.AllowedDomains); err != nil {
		return SAMLConnection{}, fmt.Errorf("store: saml connection %q domains: %w", c.ID, err)
	}
	c.Enabled, c.JITProvisioning, c.SyncRoles = enabled == 1, jit == 1, sync == 1
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return SAMLConnection{}, fmt.Errorf("store: saml connection %q created_at: %w", c.ID, err)
	}
	if c.UpdatedAt, err = time.Parse(time.RFC3339Nano, updat); err != nil {
		return SAMLConnection{}, fmt.Errorf("store: saml connection %q updated_at: %w", c.ID, err)
	}
	return c, nil
}

// ListSAMLConnections returns every connection by name.
func (db *DB) ListSAMLConnections(ctx context.Context) ([]SAMLConnection, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+samlConnectionColumns+` FROM saml_connections ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list saml connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SAMLConnection
	for rows.Next() {
		c, err := scanSAMLConnection(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list saml connections: %w", err)
	}
	return out, nil
}

// GetSAMLConnection returns one connection or ErrSAMLConnectionNotFound.
func (db *DB) GetSAMLConnection(ctx context.Context, id string) (SAMLConnection, error) {
	return scanSAMLConnection(db.QueryRowContext(ctx, `SELECT `+samlConnectionColumns+` FROM saml_connections WHERE id = ?`, id).Scan)
}

func samlConnectionArgs(c SAMLConnection) ([]any, error) {
	domains, err := json.Marshal(nonNilSlice(c.AllowedDomains))
	if err != nil {
		return nil, fmt.Errorf("store: marshal saml domains: %w", err)
	}
	return []any{c.Name, boolInt(c.Enabled), c.IDPEntityID, c.IDPSSOURL, c.IDPCertificates, c.IDPMetadataURL,
		c.EmailAttribute, c.NameAttribute, c.GroupsAttribute, string(domains), boolInt(c.JITProvisioning),
		c.DefaultRoleID, boolInt(c.SyncRoles)}, nil
}

// CreateSAMLConnection inserts c; CreatedAt and UpdatedAt are set from now.
func (db *DB) CreateSAMLConnection(ctx context.Context, c SAMLConnection, now time.Time) error {
	args, err := samlConnectionArgs(c)
	if err != nil {
		return err
	}
	args = append([]any{c.ID}, append(args, formatTime(now), formatTime(now))...)
	_, err = db.ExecContext(ctx, `INSERT INTO saml_connections (`+samlConnectionColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	if isUniqueViolation(err) {
		return ErrSAMLConnectionNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: create saml connection: %w", err)
	}
	return nil
}

// UpdateSAMLConnection rewrites every editable field of c.
func (db *DB) UpdateSAMLConnection(ctx context.Context, c SAMLConnection, now time.Time) error {
	args, err := samlConnectionArgs(c)
	if err != nil {
		return err
	}
	args = append(args, formatTime(now), c.ID)
	res, err := db.ExecContext(ctx, `UPDATE saml_connections SET name = ?, enabled = ?, idp_entity_id = ?,
		idp_sso_url = ?, idp_certificates = ?, idp_metadata_url = ?, email_attribute = ?, name_attribute = ?,
		groups_attribute = ?, allowed_domains = ?, jit_provisioning = ?, default_role_id = ?, sync_roles = ?,
		updated_at = ? WHERE id = ?`, args...)
	if isUniqueViolation(err) {
		return ErrSAMLConnectionNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: update saml connection %q: %w", c.ID, err)
	}
	return rowsAffectedOrNotFound(res, ErrSAMLConnectionNotFound, "update saml connection %q", c.ID)
}

// DeleteSAMLConnection removes a connection with its identities and pending requests.
func (db *DB) DeleteSAMLConnection(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM saml_connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete saml connection %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrSAMLConnectionNotFound, "delete saml connection %q", id)
}

// SSOSettings is the instance-wide single sign-on row.
type SSOSettings struct {
	SPCertificatePEM  string
	EnforceSSO        bool
	SCIMDefaultRoleID string
	UpdatedAt         string
}

// GetSSOSettings reads the singleton settings row.
func (db *DB) GetSSOSettings(ctx context.Context) (SSOSettings, error) {
	var s SSOSettings
	var enforce int
	err := db.QueryRowContext(ctx, `SELECT sp_cert_pem, enforce_sso, scim_default_role_id, updated_at FROM sso_settings WHERE id = 1`).
		Scan(&s.SPCertificatePEM, &enforce, &s.SCIMDefaultRoleID, &s.UpdatedAt)
	if err != nil {
		return SSOSettings{}, fmt.Errorf("store: get sso settings: %w", err)
	}
	s.EnforceSSO = enforce == 1
	return s, nil
}

// SetSSOSPCertificate stores the SP certificate only if none is stored yet and
// reports whether this call won, so two racing first uses agree on one keypair.
func (db *DB) SetSSOSPCertificate(ctx context.Context, certPEM string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE sso_settings SET sp_cert_pem = ?, updated_at = ? WHERE id = 1 AND sp_cert_pem = ''`,
		certPEM, formatTime(now))
	if err != nil {
		return false, fmt.Errorf("store: set sp certificate: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: set sp certificate: %w", err)
	}
	return n == 1, nil
}

// SetSSOEnforce turns "SSO required for non-root password sign-in" on or off.
func (db *DB) SetSSOEnforce(ctx context.Context, on bool, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE sso_settings SET enforce_sso = ?, updated_at = ? WHERE id = 1`, boolInt(on), formatTime(now)); err != nil {
		return fmt.Errorf("store: set sso enforce: %w", err)
	}
	return nil
}

// SetSCIMDefaultRole sets the role given to directory users no group mapping matches.
func (db *DB) SetSCIMDefaultRole(ctx context.Context, roleID string, now time.Time) error {
	if _, err := db.GetRole(ctx, roleID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE sso_settings SET scim_default_role_id = ?, updated_at = ? WHERE id = 1`, roleID, formatTime(now)); err != nil {
		return fmt.Errorf("store: set scim default role: %w", err)
	}
	return nil
}
