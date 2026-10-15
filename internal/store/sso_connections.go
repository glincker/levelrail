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
	// ErrSSOConnectionNotFound is returned when no connection has the given ID.
	ErrSSOConnectionNotFound = errors.New("store: saml connection not found")
	// ErrSSOConnectionNameTaken is returned when a connection name is already used.
	ErrSSOConnectionNameTaken = errors.New("store: a saml connection with that name already exists")
)

// SSOConnection is the platform side of a SAML connection (migration 0751).
// The IdP binding itself lives in the auth library under LibraryID; the
// Draft fields hold IdP settings only while LibraryID is empty.
type SSOConnection struct {
	ID              string
	LibraryID       string
	Name            string
	Enabled         bool
	IDPMetadataURL  string
	AllowedDomains  []string
	JITProvisioning bool
	DefaultRoleID   string
	SyncRoles       bool
	Draft           SSODraft
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SSODraft is an IdP binding that is not complete enough for the library yet.
type SSODraft struct {
	IDPEntityID     string
	IDPSSOURL       string
	IDPCertificate  string
	EmailAttribute  string
	NameAttribute   string
	GroupsAttribute string
}

const ssoConnectionColumns = `id, library_id, name, enabled, idp_metadata_url, allowed_domains, jit_provisioning,
	default_role_id, sync_roles, draft_idp_entity_id, draft_idp_sso_url, draft_idp_certificate,
	draft_email_attribute, draft_name_attribute, draft_groups_attribute, created_at, updated_at`

func scanSSOConnection(scan func(dest ...any) error) (SSOConnection, error) {
	var (
		c                  SSOConnection
		lib                sql.NullString
		enabled, jit, sync int
		domains, cr, up    string
	)
	d := &c.Draft
	err := scan(&c.ID, &lib, &c.Name, &enabled, &c.IDPMetadataURL, &domains, &jit, &c.DefaultRoleID, &sync,
		&d.IDPEntityID, &d.IDPSSOURL, &d.IDPCertificate, &d.EmailAttribute, &d.NameAttribute, &d.GroupsAttribute, &cr, &up)
	if errors.Is(err, sql.ErrNoRows) {
		return SSOConnection{}, ErrSSOConnectionNotFound
	}
	if err != nil {
		return SSOConnection{}, fmt.Errorf("store: scan saml connection: %w", err)
	}
	c.LibraryID = lib.String
	if err := json.Unmarshal([]byte(domains), &c.AllowedDomains); err != nil {
		return SSOConnection{}, fmt.Errorf("store: saml connection %q domains: %w", c.ID, err)
	}
	c.Enabled, c.JITProvisioning, c.SyncRoles = enabled == 1, jit == 1, sync == 1
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, cr); err != nil {
		return SSOConnection{}, fmt.Errorf("store: saml connection %q created_at: %w", c.ID, err)
	}
	if c.UpdatedAt, err = time.Parse(time.RFC3339Nano, up); err != nil {
		return SSOConnection{}, fmt.Errorf("store: saml connection %q updated_at: %w", c.ID, err)
	}
	return c, nil
}

// ListSSOConnections returns every connection by name.
func (db *DB) ListSSOConnections(ctx context.Context) ([]SSOConnection, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+ssoConnectionColumns+` FROM sso_saml_connections ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list saml connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SSOConnection
	for rows.Next() {
		c, err := scanSSOConnection(rows.Scan)
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

// GetSSOConnection returns one connection or ErrSSOConnectionNotFound.
func (db *DB) GetSSOConnection(ctx context.Context, id string) (SSOConnection, error) {
	return scanSSOConnection(db.QueryRowContext(ctx, `SELECT `+ssoConnectionColumns+` FROM sso_saml_connections WHERE id = ?`, id).Scan)
}

func ssoConnectionArgs(c SSOConnection) ([]any, error) {
	domains, err := json.Marshal(nonNilSlice(c.AllowedDomains))
	if err != nil {
		return nil, fmt.Errorf("store: marshal saml domains: %w", err)
	}
	var lib any
	if c.LibraryID != "" {
		lib = c.LibraryID
	}
	d := c.Draft
	return []any{lib, c.Name, boolInt(c.Enabled), c.IDPMetadataURL, string(domains), boolInt(c.JITProvisioning),
		c.DefaultRoleID, boolInt(c.SyncRoles), d.IDPEntityID, d.IDPSSOURL, d.IDPCertificate,
		d.EmailAttribute, d.NameAttribute, d.GroupsAttribute}, nil
}

// SaveSSOConnection inserts c when create is set, else rewrites it.
func (db *DB) SaveSSOConnection(ctx context.Context, c SSOConnection, create bool, now time.Time) error {
	args, err := ssoConnectionArgs(c)
	if err != nil {
		return err
	}
	var res sql.Result
	if create {
		args = append([]any{c.ID}, append(args, formatTime(now), formatTime(now))...)
		res, err = db.ExecContext(ctx, `INSERT INTO sso_saml_connections (`+ssoConnectionColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	} else {
		args = append(args, formatTime(now), c.ID)
		res, err = db.ExecContext(ctx, `UPDATE sso_saml_connections SET library_id = ?, name = ?, enabled = ?,
			idp_metadata_url = ?, allowed_domains = ?, jit_provisioning = ?, default_role_id = ?, sync_roles = ?,
			draft_idp_entity_id = ?, draft_idp_sso_url = ?, draft_idp_certificate = ?, draft_email_attribute = ?,
			draft_name_attribute = ?, draft_groups_attribute = ?, updated_at = ? WHERE id = ?`, args...)
	}
	if isUniqueViolation(err) {
		return ErrSSOConnectionNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: save saml connection %q: %w", c.ID, err)
	}
	return rowsAffectedOrNotFound(res, ErrSSOConnectionNotFound, "save saml connection %q", c.ID)
}

// DeleteSSOConnection removes the platform side of a connection.
func (db *DB) DeleteSSOConnection(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM sso_saml_connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete saml connection %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrSSOConnectionNotFound, "delete saml connection %q", id)
}
