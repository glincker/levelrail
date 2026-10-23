package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
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
	// ExpiresAt is operator-provided, never inferred: this platform has
	// no way to read an expiry out of an opaque credential string. Nil
	// means no expiry was set.
	ExpiresAt *time.Time
	// Preset is "", "ecr", "artifact_registry" or "acr". A preset credential's
	// password is minted and rotated by the refresher, never typed in.
	Preset string
	// PresetConfig is non-secret JSON (region and similar) for the preset.
	PresetConfig     string
	LastRefreshedAt  *time.Time
	NextRefreshAt    *time.Time
	LastRefreshError string
}

const registryCredentialColumns = //nolint:gosec // column list, not a credential
`id, name, registry_host, username, created_at, expires_at, preset, preset_config, last_refreshed_at, next_refresh_at, last_refresh_error`

func scanRegistryCredential(row rowScanner) (RegistryCredential, error) {
	var c RegistryCredential
	var expiresAt, refreshed, next sql.NullString
	if err := row.Scan(&c.ID, &c.Name, &c.RegistryHost, &c.Username, &c.CreatedAt, &expiresAt, &c.Preset, &c.PresetConfig, &refreshed, &next, &c.LastRefreshError); err != nil {
		return RegistryCredential{}, err
	}
	var err error
	if c.ExpiresAt, err = parseTimePtr(expiresAt); err != nil {
		return RegistryCredential{}, fmt.Errorf("parse expires_at for registry credential %q: %w", c.ID, err)
	}
	if c.LastRefreshedAt, err = parseTimePtr(refreshed); err != nil {
		return RegistryCredential{}, fmt.Errorf("parse last_refreshed_at for registry credential %q: %w", c.ID, err)
	}
	if c.NextRefreshAt, err = parseTimePtr(next); err != nil {
		return RegistryCredential{}, fmt.Errorf("parse next_refresh_at for registry credential %q: %w", c.ID, err)
	}
	return c, nil
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
		INSERT INTO registry_credentials (id, name, registry_host, username, created_at, expires_at, preset, preset_config, last_refreshed_at, next_refresh_at, last_refresh_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Name, c.RegistryHost, c.Username, c.CreatedAt, formatTimePtr(c.ExpiresAt), c.Preset, c.PresetConfig, formatTimePtr(c.LastRefreshedAt), formatTimePtr(c.NextRefreshAt), c.LastRefreshError)
	if err != nil {
		return fmt.Errorf("store: save registry credential %q: %w", c.ID, err)
	}
	return nil
}

// GetRegistryCredential returns the registry credential with this ID, or
// ErrRegistryCredentialNotFound.
func (db *DB) GetRegistryCredential(ctx context.Context, id string) (RegistryCredential, error) {
	c, err := scanRegistryCredential(db.QueryRowContext(ctx, `SELECT `+registryCredentialColumns+` FROM registry_credentials WHERE id = ?`, id))
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
	c, err := scanRegistryCredential(db.QueryRowContext(ctx, `SELECT `+registryCredentialColumns+` FROM registry_credentials WHERE name = ?`, name))
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
	rows, err := db.QueryContext(ctx, `SELECT `+registryCredentialColumns+` FROM registry_credentials ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list registry credentials: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []RegistryCredential
	for rows.Next() {
		c, err := scanRegistryCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan registry credential row: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate registry credential rows: %w", err)
	}
	return out, nil
}

// UpdateRegistryCredential replaces name/registry_host/username/expiresAt
// for an existing row, the same full-replace contract UpdateBackupTarget
// uses for its own sibling resource. Rotating the password is a separate
// write to internal/secrets under this row's existing
// RegistryCredentialSecretsKey, not this function's concern. Returns
// ErrRegistryCredentialNotFound if id doesn't exist.
func (db *DB) UpdateRegistryCredential(ctx context.Context, id, name, registryHost, username string, expiresAt *time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE registry_credentials
		SET name = ?, registry_host = ?, username = ?, expires_at = ?
		WHERE id = ?
	`, name, registryHost, username, formatTimePtr(expiresAt), id)
	if err != nil {
		return fmt.Errorf("store: update registry credential %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrRegistryCredentialNotFound, "update registry credential %q", id)
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

// RecordRegistryCredentialRefresh stores one refresh outcome in a single
// UPDATE. On success pass the new host, username and expiry with an empty
// errMsg; on failure pass errMsg and nil times so the old expiry stays.
func (db *DB) RecordRegistryCredentialRefresh(ctx context.Context, id, registryHost, username string, expiresAt, refreshedAt, nextAt *time.Time, errMsg string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE registry_credentials
		SET registry_host = CASE WHEN ? != '' THEN ? ELSE registry_host END,
		    username = CASE WHEN ? != '' THEN ? ELSE username END,
		    expires_at = COALESCE(?, expires_at),
		    last_refreshed_at = COALESCE(?, last_refreshed_at),
		    next_refresh_at = ?,
		    last_refresh_error = ?
		WHERE id = ?
	`, registryHost, registryHost, username, username, formatTimePtr(expiresAt), formatTimePtr(refreshedAt), formatTimePtr(nextAt), errMsg, id)
	if err != nil {
		return fmt.Errorf("store: record registry credential refresh %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrRegistryCredentialNotFound, "record registry credential refresh %q", id)
}
