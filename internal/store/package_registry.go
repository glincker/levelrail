package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PackageRegistry is the non-secret half of a private package registry
// credential. The token is stored under PackageRegistrySecretsKey.
type PackageRegistry struct {
	ID             string
	Name           string
	Ecosystem      string
	URL            string
	Username       string
	ModulePatterns string
	ScopeKind      string
	ScopeRef       string
	CreatedBy      string
	CreatedAt      time.Time
}

// ErrPackageRegistryNotFound is returned when no row matches.
var ErrPackageRegistryNotFound = errors.New("store: package registry not found")

// PackageRegistrySecretsKey is the secrets serviceName for one registry.
func PackageRegistrySecretsKey(id string) string { return "package-registry/" + id }

// PackageRegistrySecretEnvKey is the fixed envKey within that namespace.
const PackageRegistrySecretEnvKey = "secret"

const packageRegistryColumns = "id, name, ecosystem, url, username, module_patterns, scope_kind, scope_ref, created_by, created_at"

// SavePackageRegistry inserts a new row.
func (db *DB) SavePackageRegistry(ctx context.Context, p PackageRegistry) error {
	_, err := db.ExecContext(ctx, `INSERT INTO package_registries (`+packageRegistryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Ecosystem, p.URL, p.Username, p.ModulePatterns, p.ScopeKind, p.ScopeRef, p.CreatedBy, formatTime(p.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: save package registry %q: %w", p.Name, err)
	}
	return nil
}

// GetPackageRegistry returns one row or ErrPackageRegistryNotFound.
func (db *DB) GetPackageRegistry(ctx context.Context, id string) (PackageRegistry, error) {
	row := db.QueryRowContext(ctx, `SELECT `+packageRegistryColumns+` FROM package_registries WHERE id = ?`, id)
	p, err := scanPackageRegistry(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PackageRegistry{}, ErrPackageRegistryNotFound
	}
	if err != nil {
		return PackageRegistry{}, fmt.Errorf("store: get package registry %q: %w", id, err)
	}
	return p, nil
}

// ListPackageRegistries returns every row, newest first.
func (db *DB) ListPackageRegistries(ctx context.Context) ([]PackageRegistry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+packageRegistryColumns+` FROM package_registries ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list package registries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PackageRegistry
	for rows.Next() {
		p, err := scanPackageRegistry(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan package registry: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate package registries: %w", err)
	}
	return out, nil
}

// DeletePackageRegistry removes one row.
func (db *DB) DeletePackageRegistry(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM package_registries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete package registry %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrPackageRegistryNotFound, "delete package registry %q", id)
}

// ListServicesUsingRegistryCredential returns the names of services that
// pull with this registry credential.
func (db *DB) ListServicesUsingRegistryCredential(ctx context.Context, credentialID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM desired_services WHERE registry_credential_id = ? ORDER BY name`, credentialID)
	if err != nil {
		return nil, fmt.Errorf("store: list services using registry credential %q: %w", credentialID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan service name: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate services: %w", err)
	}
	return out, nil
}

func scanPackageRegistry(scan func(dest ...any) error) (PackageRegistry, error) {
	var (
		p         PackageRegistry
		createdAt string
	)
	if err := scan(&p.ID, &p.Name, &p.Ecosystem, &p.URL, &p.Username, &p.ModulePatterns, &p.ScopeKind, &p.ScopeRef, &p.CreatedBy, &createdAt); err != nil {
		return PackageRegistry{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return PackageRegistry{}, fmt.Errorf("parse created_at: %w", err)
	}
	p.CreatedAt = t
	return p, nil
}
