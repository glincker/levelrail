package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DomainBasicAuthSecretsKey is the internal/secrets serviceName a
// domain's basic-auth password is stored under (envKey
// DomainBasicAuthPasswordEnvKey), mirroring BackupTargetSecretsKey's
// "<kind>/<id>" shape, keyed by domain instead of a target ID.
func DomainBasicAuthSecretsKey(domain string) string {
	return "domain-basic-auth/" + domain
}

// DomainBasicAuthPasswordEnvKey is the fixed envKey used within that
// namespace, mirroring CloudflareTunnelTokenEnvKey's shape.
const DomainBasicAuthPasswordEnvKey = "password"

// DomainBasicAuth is one row of the domain_basic_auth table
// (migrations/0052_domain_basic_auth.sql): a username claimed for a
// domain already present in service_domains. No password field: that
// goes through internal/secrets instead, the same split
// CloudflareTunnelSettings uses for its own token.
type DomainBasicAuth struct {
	Domain   string
	Username string
}

// GetDomainBasicAuth returns the basic-auth row for domain. found is
// false when the domain has none configured, the default, valid state
// for any domain, not an error.
func (db *DB) GetDomainBasicAuth(ctx context.Context, domain string) (DomainBasicAuth, bool, error) {
	var d DomainBasicAuth
	err := db.QueryRowContext(ctx, `
		SELECT domain, username FROM domain_basic_auth WHERE domain = ?
	`, domain).Scan(&d.Domain, &d.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return DomainBasicAuth{}, false, nil
	}
	if err != nil {
		return DomainBasicAuth{}, false, fmt.Errorf("store: get domain basic auth: %w", err)
	}
	return d, true, nil
}

// SetDomainBasicAuth upserts the username claimed for domain. Callers
// (internal/api) are expected to write the matching password through
// internal/secrets in the same request; this method only ever tracks
// the username, never a credential.
func (db *DB) SetDomainBasicAuth(ctx context.Context, domain, username string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_basic_auth (domain, username) VALUES (?, ?)
		ON CONFLICT (domain) DO UPDATE SET username = excluded.username
	`, domain, username)
	if err != nil {
		return fmt.Errorf("store: set domain basic auth: %w", err)
	}
	return nil
}

// DeleteDomainBasicAuth removes domain's basic-auth row, if any.
// Idempotent: deleting a domain with none configured is not an error.
func (db *DB) DeleteDomainBasicAuth(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_basic_auth WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain basic auth: %w", err)
	}
	return nil
}

// ListDomainBasicAuth returns every domain_basic_auth row, ordered by
// domain, for the ingress controller to build basic_auth directives
// fresh every reconcile pass, the same "never cache, re-derive from
// current state every call" convention ListServiceDomains already
// follows.
func (db *DB) ListDomainBasicAuth(ctx context.Context) ([]DomainBasicAuth, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, username FROM domain_basic_auth ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain basic auth: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DomainBasicAuth
	for rows.Next() {
		var d DomainBasicAuth
		if err := rows.Scan(&d.Domain, &d.Username); err != nil {
			return nil, fmt.Errorf("store: scan domain basic auth row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain basic auth rows: %w", err)
	}
	return out, nil
}
