package store

import (
	"context"
	"fmt"
)

// CloudflareDNSSecretsKey is the internal/secrets serviceName the
// Cloudflare DNS-01 API token is stored under (envKey
// CloudflareDNSTokenEnvKey), mirroring CloudflareTunnelSecretsKey's
// shape but for a distinct credential: a scoped Cloudflare API token
// (Zone:DNS:Edit), not the cloudflared connector token.
func CloudflareDNSSecretsKey() string {
	return "cloudflare-dns"
}

// CloudflareDNSTokenEnvKey is the fixed envKey used within that namespace.
const CloudflareDNSTokenEnvKey = "token"

// CloudflareDNSSettings is the single platform-wide row: whether ACME
// DNS-01 via Cloudflare is enabled for wildcard domains. No token
// field: that goes through internal/secrets instead.
type CloudflareDNSSettings struct {
	Enabled bool
}

// GetCloudflareDNSSettings returns the single cloudflare_dns_settings
// row. Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetCloudflareDNSSettings(ctx context.Context) (CloudflareDNSSettings, error) {
	var enabled int
	err := db.QueryRowContext(ctx, `
		SELECT enabled FROM cloudflare_dns_settings WHERE id = 1
	`).Scan(&enabled)
	if err != nil {
		return CloudflareDNSSettings{}, fmt.Errorf("store: get cloudflare dns settings: %w", err)
	}
	return CloudflareDNSSettings{Enabled: enabled != 0}, nil
}

// UpdateCloudflareDNSSettings replaces the single row's Enabled flag.
func (db *DB) UpdateCloudflareDNSSettings(ctx context.Context, s CloudflareDNSSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE cloudflare_dns_settings SET enabled = ? WHERE id = 1
	`, enabled)
	if err != nil {
		return fmt.Errorf("store: update cloudflare dns settings: %w", err)
	}
	return nil
}
