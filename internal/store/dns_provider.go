package store

import (
	"context"
	"fmt"
)

// DNSProviderChoice is a wildcard-DNS provider selection; Provider is empty for "no override".
type DNSProviderChoice struct {
	Provider         string
	CustomBaseDomain string
}

// GetDNSProviderDefault returns the instance-wide provider choice (migrations/2660).
func (db *DB) GetDNSProviderDefault(ctx context.Context) (DNSProviderChoice, error) {
	var c DNSProviderChoice
	err := db.QueryRowContext(ctx, `SELECT provider, custom_base_domain FROM dns_provider_default WHERE id = 1`).Scan(&c.Provider, &c.CustomBaseDomain)
	if err != nil {
		return DNSProviderChoice{}, fmt.Errorf("store: get dns provider default: %w", err)
	}
	return c, nil
}

// SetDNSProviderDefault replaces the instance-wide provider choice.
func (db *DB) SetDNSProviderDefault(ctx context.Context, c DNSProviderChoice) error {
	if _, err := db.ExecContext(ctx, `UPDATE dns_provider_default SET provider = ?, custom_base_domain = ? WHERE id = 1`, c.Provider, c.CustomBaseDomain); err != nil {
		return fmt.Errorf("store: set dns provider default: %w", err)
	}
	return nil
}

// GetAppDNSProvider returns app's override, ok false when it follows the default.
func (db *DB) GetAppDNSProvider(ctx context.Context, app string) (DNSProviderChoice, bool, error) {
	var c DNSProviderChoice
	rows, err := db.QueryContext(ctx, `SELECT provider, custom_base_domain FROM app_dns_provider WHERE app_name = ?`, app)
	if err != nil {
		return c, false, fmt.Errorf("store: get app dns provider: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return c, false, rows.Err()
	}
	if err := rows.Scan(&c.Provider, &c.CustomBaseDomain); err != nil {
		return c, false, fmt.Errorf("store: scan app dns provider: %w", err)
	}
	return c, true, nil
}

// SetAppDNSProvider stores app's override; an empty Provider removes it.
func (db *DB) SetAppDNSProvider(ctx context.Context, app string, c DNSProviderChoice) error {
	if c.Provider == "" {
		if _, err := db.ExecContext(ctx, `DELETE FROM app_dns_provider WHERE app_name = ?`, app); err != nil {
			return fmt.Errorf("store: clear app dns provider: %w", err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_dns_provider (app_name, provider, custom_base_domain) VALUES (?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET provider = excluded.provider, custom_base_domain = excluded.custom_base_domain
	`, app, c.Provider, c.CustomBaseDomain)
	if err != nil {
		return fmt.Errorf("store: set app dns provider: %w", err)
	}
	return nil
}

// ListAppDNSProviders returns every per-app override keyed by app name.
func (db *DB) ListAppDNSProviders(ctx context.Context) (map[string]DNSProviderChoice, error) {
	rows, err := db.QueryContext(ctx, `SELECT app_name, provider, custom_base_domain FROM app_dns_provider`)
	if err != nil {
		return nil, fmt.Errorf("store: list app dns providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]DNSProviderChoice{}
	for rows.Next() {
		var app string
		var c DNSProviderChoice
		if err := rows.Scan(&app, &c.Provider, &c.CustomBaseDomain); err != nil {
			return nil, fmt.Errorf("store: scan app dns provider: %w", err)
		}
		out[app] = c
	}
	return out, rows.Err()
}
