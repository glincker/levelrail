package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by the secret connection store.
var (
	ErrSecretConnectionExists   = errors.New("store: secret connection already exists")
	ErrSecretConnectionNotFound = errors.New("store: secret connection not found")
)

// SecretConnection is one named external secret source. Config is opaque JSON
// of non-secret settings; the credential lives in the encrypted secret store,
// or CredentialRef points at an existing entry there.
type SecretConnection struct {
	Name          string
	Type          string
	Config        string
	CredentialRef string
	// Policy is JSON of the allowed path prefixes and environments; empty or {} means no allow list.
	Policy    string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const secretConnectionCols = "name, type, config, credential_ref, policy, created_by, created_at, updated_at" //nolint:gosec // route and column names, not credentials

func scanSecretConnection(sc interface{ Scan(...any) error }) (SecretConnection, error) {
	var c SecretConnection
	var created, updated string
	if err := sc.Scan(&c.Name, &c.Type, &c.Config, &c.CredentialRef, &c.Policy, &c.CreatedBy, &created, &updated); err != nil {
		return SecretConnection{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, nil
}

// ListSecretConnections returns every connection ordered by name.
func (db *DB) ListSecretConnections(ctx context.Context) ([]SecretConnection, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+secretConnectionCols+" FROM secret_connections ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("store: list secret connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []SecretConnection{}
	for rows.Next() {
		c, err := scanSecretConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan secret connection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetSecretConnection returns one connection or ErrSecretConnectionNotFound.
func (db *DB) GetSecretConnection(ctx context.Context, name string) (SecretConnection, error) {
	c, err := scanSecretConnection(db.QueryRowContext(ctx, "SELECT "+secretConnectionCols+" FROM secret_connections WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return SecretConnection{}, ErrSecretConnectionNotFound
	}
	if err != nil {
		return SecretConnection{}, fmt.Errorf("store: get secret connection: %w", err)
	}
	return c, nil
}

// CreateSecretConnection inserts c, or returns ErrSecretConnectionExists.
func (db *DB) CreateSecretConnection(ctx context.Context, c SecretConnection) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `INSERT INTO secret_connections (`+secretConnectionCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Type, c.Config, c.CredentialRef, normalizePolicyJSON(c.Policy), c.CreatedBy, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return ErrSecretConnectionExists
		}
		return fmt.Errorf("store: create secret connection: %w", err)
	}
	return nil
}

// UpdateSecretConnection replaces config, credential_ref and policy; type and creator never change.
func (db *DB) UpdateSecretConnection(ctx context.Context, c SecretConnection) error {
	res, err := db.ExecContext(ctx, `UPDATE secret_connections SET config = ?, credential_ref = ?, policy = ?, updated_at = ? WHERE name = ?`,
		c.Config, c.CredentialRef, normalizePolicyJSON(c.Policy), time.Now().UTC().Format(time.RFC3339Nano), c.Name)
	if err != nil {
		return fmt.Errorf("store: update secret connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretConnectionNotFound
	}
	return nil
}

// DeleteSecretConnection removes a connection; a missing one is ErrSecretConnectionNotFound.
func (db *DB) DeleteSecretConnection(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM secret_connections WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete secret connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretConnectionNotFound
	}
	return nil
}

func normalizePolicyJSON(p string) string {
	if strings.TrimSpace(p) == "" {
		return "{}"
	}
	return p
}
