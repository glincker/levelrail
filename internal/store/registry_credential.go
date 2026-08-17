package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrRegistryCredentialNotFound is returned by GetRegistryCredential and
// DeleteRegistryCredential when id doesn't match any row.
var ErrRegistryCredentialNotFound = errors.New("store: registry credential not found")

// RegistryCredential is a username for pulling a private container image
// from RegistryHost. No password field: a caller resolving it does so
// separately through internal/secrets, keyed by
// RegistryCredentialSecretsKey(credential.ID), the same split
// BackupTarget already uses.
type RegistryCredential struct {
	ID           string
	Name         string
	RegistryHost string
	Username     string
	CreatedAt    string
}

// RegistryCredentialSecretsKey is the internal/secrets serviceName a
// registry credential's password is stored under (envKey "password").
func RegistryCredentialSecretsKey(id string) string {
	return "registry-credential/" + id
}

// SaveRegistryCredential inserts a new registry credential row. IDs are
// minted by the caller before this call, the same "generate before the
// INSERT" pattern BackupTarget uses.
func (db *DB) SaveRegistryCredential(ctx context.Context, c RegistryCredential) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO registry_credentials (id, name, registry_host, username, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, c.ID, c.Name, c.RegistryHost, c.Username, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save registry credential %q: %w", c.ID, err)
	}
	return nil
}

// GetRegistryCredential returns the registry credential with this ID, or
// ErrRegistryCredentialNotFound.
func (db *DB) GetRegistryCredential(ctx context.Context, id string) (RegistryCredential, error) {
	var c RegistryCredential
	err := db.QueryRowContext(ctx, `
		SELECT id, name, registry_host, username, created_at
		FROM registry_credentials
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.RegistryHost, &c.Username, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RegistryCredential{}, ErrRegistryCredentialNotFound
	}
	if err != nil {
		return RegistryCredential{}, fmt.Errorf("store: get registry credential %q: %w", id, err)
	}
	return c, nil
}

// GetRegistryCredentialByName returns the registry credential with this
// Name, or ErrRegistryCredentialNotFound. app.yaml's build.type: image
// references a credential by Name (spec.Build.RegistryCredential's own
// doc comment: an opaque ID would be hostile to hand-author), so a
// deploy resolves it this way, not by ID.
func (db *DB) GetRegistryCredentialByName(ctx context.Context, name string) (RegistryCredential, error) {
	var c RegistryCredential
	err := db.QueryRowContext(ctx, `
		SELECT id, name, registry_host, username, created_at
		FROM registry_credentials
		WHERE name = ?
	`, name).Scan(&c.ID, &c.Name, &c.RegistryHost, &c.Username, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RegistryCredential{}, ErrRegistryCredentialNotFound
	}
	if err != nil {
		return RegistryCredential{}, fmt.Errorf("store: get registry credential by name %q: %w", name, err)
	}
	return c, nil
}

// ListRegistryCredentials returns every registry credential, oldest
// first.
func (db *DB) ListRegistryCredentials(ctx context.Context) ([]RegistryCredential, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, registry_host, username, created_at
		FROM registry_credentials
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list registry credentials: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []RegistryCredential
	for rows.Next() {
		var c RegistryCredential
		if err := rows.Scan(&c.ID, &c.Name, &c.RegistryHost, &c.Username, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan registry credential row: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate registry credential rows: %w", err)
	}
	return out, nil
}

// DeleteRegistryCredential removes a registry credential row. It does
// not touch internal/secrets: the caller deletes the password secret
// separately, after this succeeds, the same ordering BackupTarget uses.
// Returns ErrRegistryCredentialNotFound if id doesn't exist.
func (db *DB) DeleteRegistryCredential(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM registry_credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete registry credential %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete registry credential %q: %w", id, err)
	}
	if n == 0 {
		return ErrRegistryCredentialNotFound
	}
	return nil
}
