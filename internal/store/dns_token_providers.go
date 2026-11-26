package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DNSTokenSecretsKey is the internal/secrets serviceName a token DNS
// provider's credential is stored under.
func DNSTokenSecretsKey(provider string) string {
	return "dns-" + provider
}

// DNSTokenEnvKey is the envKey of the stored credential.
const DNSTokenEnvKey = "credential"

// GetDNSTokenProviderEnabled reports whether the provider is enabled.
func (db *DB) GetDNSTokenProviderEnabled(ctx context.Context, provider string) (bool, error) {
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT enabled FROM dns_token_providers WHERE provider = ?`, provider).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		// A provider without a row (e.g. an install that predates its seed)
		// is simply not configured: resolution must skip it, not abort.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: get dns provider %s: %w", provider, err)
	}
	return enabled != 0, nil
}

// SetDNSTokenProviderEnabled turns the provider on or off.
func (db *DB) SetDNSTokenProviderEnabled(ctx context.Context, provider string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := db.ExecContext(ctx, `UPDATE dns_token_providers SET enabled = ? WHERE provider = ?`, v, provider)
	if err != nil {
		return fmt.Errorf("store: set dns provider %s: %w", provider, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: unknown dns provider %q", provider)
	}
	return nil
}
