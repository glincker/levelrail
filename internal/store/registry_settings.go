package store

import (
	"context"
	"database/sql"
	"fmt"
)

// RegistrySettingsSecretsKey is the internal/secrets serviceName the
// built-in registry's generated password is stored under (envKey
// RegistryPasswordEnvKey), mirroring CloudflareTunnelSecretsKey's exact
// shape: one platform-wide credential, not per-app.
func RegistrySettingsSecretsKey() string {
	return "builtin-registry"
}

// RegistryPasswordEnvKey is the fixed envKey used within that namespace.
const RegistryPasswordEnvKey = "password"

// RegistrySettings is the single platform-wide row: whether the operator
// wants Levelrail's built-in container registry running, the hostname it
// should be reachable at, and the generated login username. No password
// field: that goes through internal/secrets instead.
type RegistrySettings struct {
	Enabled bool
	// Host is the domain the embedded Caddy ingress routes to the
	// registry container, e.g. a WireGuard mesh-resolvable name for
	// multi-node reachability, or any hostname on a single node. Empty
	// means "not set yet": Enabled with an empty Host reconciles the
	// container but never routes it, the same "known, incomplete
	// configuration" shape IngressSettings.PrimaryDomain's absence
	// already has for the dashboard route.
	Host string
	// Username is the registry login name, generated once alongside the
	// password when the registry is first enabled.
	Username  string
	CreatedAt string
}

// GetRegistrySettings returns the single registry_settings row. Always
// succeeds against a migrated database: the migration itself inserts the
// row (id = 1).
func (db *DB) GetRegistrySettings(ctx context.Context) (RegistrySettings, error) {
	var (
		s         RegistrySettings
		host      sql.NullString
		username  sql.NullString
		createdAt sql.NullString
		enabled   int
	)
	err := db.QueryRowContext(ctx, `
		SELECT enabled, host, username, created_at FROM registry_settings WHERE id = 1
	`).Scan(&enabled, &host, &username, &createdAt)
	if err != nil {
		return RegistrySettings{}, fmt.Errorf("store: get registry settings: %w", err)
	}
	s.Enabled = enabled != 0
	s.Host = host.String
	s.Username = username.String
	s.CreatedAt = createdAt.String
	return s, nil
}

// UpdateRegistrySettings replaces the single registry_settings row in
// full, the same whole-record convention UpdateCloudflareTunnelSettings
// already establishes.
func (db *DB) UpdateRegistrySettings(ctx context.Context, s RegistrySettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE registry_settings SET enabled = ?, host = ?, username = ?, created_at = ? WHERE id = 1
	`,
		enabled,
		sql.NullString{String: s.Host, Valid: s.Host != ""},
		sql.NullString{String: s.Username, Valid: s.Username != ""},
		sql.NullString{String: s.CreatedAt, Valid: s.CreatedAt != ""},
	)
	if err != nil {
		return fmt.Errorf("store: update registry settings: %w", err)
	}
	return nil
}
