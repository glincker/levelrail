package store

import (
	"context"
	"fmt"
)

// CloudflareTunnelSecretsKey is the internal/secrets serviceName the
// Cloudflare Tunnel token is stored under (envKey
// CloudflareTunnelTokenEnvKey), mirroring EmailSettingsSecretsKey's exact
// shape: one platform-wide credential, not per-app.
func CloudflareTunnelSecretsKey() string {
	return "cloudflare-tunnel"
}

// CloudflareTunnelTokenEnvKey is the fixed envKey used within that
// namespace, mirroring OAuthProviderSecretEnvKey's shape.
const CloudflareTunnelTokenEnvKey = "token"

// CloudflareTunnelSettings is the single platform-wide row: whether the
// operator wants the cloudflared container running. No token field: that
// goes through internal/secrets instead.
type CloudflareTunnelSettings struct {
	Enabled bool
}

// GetCloudflareTunnelSettings returns the single cloudflare_tunnel_settings
// row. Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetCloudflareTunnelSettings(ctx context.Context) (CloudflareTunnelSettings, error) {
	var enabled int
	err := db.QueryRowContext(ctx, `
		SELECT enabled FROM cloudflare_tunnel_settings WHERE id = 1
	`).Scan(&enabled)
	if err != nil {
		return CloudflareTunnelSettings{}, fmt.Errorf("store: get cloudflare tunnel settings: %w", err)
	}
	return CloudflareTunnelSettings{Enabled: enabled != 0}, nil
}

// UpdateCloudflareTunnelSettings replaces the single row's Enabled flag,
// the same whole-record convention UpdateEmailSettings uses.
func (db *DB) UpdateCloudflareTunnelSettings(ctx context.Context, s CloudflareTunnelSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE cloudflare_tunnel_settings SET enabled = ? WHERE id = 1
	`, enabled)
	if err != nil {
		return fmt.Errorf("store: update cloudflare tunnel settings: %w", err)
	}
	return nil
}
