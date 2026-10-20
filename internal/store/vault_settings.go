package store

import (
	"context"
	"fmt"
)

// VaultAuthMethodToken and VaultAuthMethodAppRole are
// VaultSettings.AuthMethod's only valid values.
const (
	VaultAuthMethodToken   = "token"
	VaultAuthMethodAppRole = "approle"
)

// VaultSecretsKey is the internal/secrets serviceName the Vault
// credential is stored under (envKey VaultCredentialEnvKey), mirroring
// CloudflareTunnelSecretsKey's exact shape: one platform-wide
// credential, not per-app.
func VaultSecretsKey() string {
	return "vault"
}

// VaultCredentialEnvKey is the fixed envKey used within that namespace.
// It holds a Vault token when AuthMethod is VaultAuthMethodToken, or an
// AppRole secret ID when AuthMethod is VaultAuthMethodAppRole: exactly
// one credential is ever configured at a time, so one envKey covers
// both shapes.
const VaultCredentialEnvKey = "credential"

// VaultSettings is the single platform-wide row: how to reach an
// external HashiCorp Vault instance for Vault-sourced app secrets
// (internal/spec.EnvVar.Vault). RoleID is only meaningful alongside
// VaultAuthMethodAppRole; the credential itself (token or AppRole
// secret ID) goes through internal/secrets instead, keyed by
// VaultSecretsKey/VaultCredentialEnvKey.
type VaultSettings struct {
	Enabled    bool
	Address    string
	AuthMethod string
	Namespace  string
	RoleID     string
	MountPath  string
}

// GetVaultSettings returns the single vault_settings row. Always
// succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetVaultSettings(ctx context.Context) (VaultSettings, error) {
	var (
		enabled int
		s       VaultSettings
	)
	err := db.QueryRowContext(ctx, `
		SELECT enabled, address, auth_method, namespace, role_id, mount_path
		FROM vault_settings WHERE id = 1
	`).Scan(&enabled, &s.Address, &s.AuthMethod, &s.Namespace, &s.RoleID, &s.MountPath)
	if err != nil {
		return VaultSettings{}, fmt.Errorf("store: get vault settings: %w", err)
	}
	s.Enabled = enabled != 0
	return s, nil
}

// VaultConfigured reports whether an operator has enabled Vault
// integration (VaultSettings.Enabled), the narrow check
// internal/deploy.VaultConfigChecker needs at deploy validation time
// without exposing the whole settings row.
func (db *DB) VaultConfigured(ctx context.Context) (bool, error) {
	s, err := db.GetVaultSettings(ctx)
	if err != nil {
		return false, fmt.Errorf("store: check vault configured: %w", err)
	}
	return s.Enabled, nil
}

// UpdateVaultSettings replaces the single row, the same whole-record
// convention UpdateCloudflareTunnelSettings uses.
func (db *DB) UpdateVaultSettings(ctx context.Context, s VaultSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	mountPath := s.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}
	_, err := db.ExecContext(ctx, `
		UPDATE vault_settings
		SET enabled = ?, address = ?, auth_method = ?, namespace = ?, role_id = ?, mount_path = ?
		WHERE id = 1
	`, enabled, s.Address, s.AuthMethod, s.Namespace, s.RoleID, mountPath)
	if err != nil {
		return fmt.Errorf("store: update vault settings: %w", err)
	}
	return nil
}
