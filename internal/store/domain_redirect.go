package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Domain redirect status codes (domain_redirect.status_code, migrations/0102).
// DomainRedirectPermanent is the default, matching Caddy's own default
// redirect status.
const (
	DomainRedirectPermanent = 301
	DomainRedirectTemporary = 302
)

// DomainRedirect is one row of the domain_redirect table: an opt-in
// redirect for a domain already present in service_domains, to an
// arbitrary absolute target URL.
type DomainRedirect struct {
	Domain     string
	TargetURL  string
	StatusCode int
}

// GetDomainRedirect returns domain's current redirect configuration.
// found is false when the domain has no row, the default, valid state
// for any domain, not an error.
func (db *DB) GetDomainRedirect(ctx context.Context, domain string) (DomainRedirect, bool, error) {
	var d DomainRedirect
	err := db.QueryRowContext(ctx, `
		SELECT domain, target_url, status_code FROM domain_redirect WHERE domain = ?
	`, domain).Scan(&d.Domain, &d.TargetURL, &d.StatusCode)
	if errors.Is(err, sql.ErrNoRows) {
		return DomainRedirect{Domain: domain, StatusCode: DomainRedirectPermanent}, false, nil
	}
	if err != nil {
		return DomainRedirect{}, false, fmt.Errorf("store: get domain redirect: %w", err)
	}
	return d, true, nil
}

// SetDomainRedirect upserts domain's redirect configuration. targetURL
// and statusCode must already be validated by the caller (internal/api);
// this method performs no validation of its own, the same division of
// responsibility store.SetDomainWAF already draws with its own caller.
func (db *DB) SetDomainRedirect(ctx context.Context, domain, targetURL string, statusCode int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_redirect (domain, target_url, status_code)
		VALUES (?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET
			target_url = excluded.target_url,
			status_code = excluded.status_code
	`, domain, targetURL, statusCode)
	if err != nil {
		return fmt.Errorf("store: set domain redirect: %w", err)
	}
	return nil
}

// DeleteDomainRedirect removes domain's redirect row, if any. Idempotent:
// deleting a domain with none configured is not an error.
func (db *DB) DeleteDomainRedirect(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_redirect WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain redirect: %w", err)
	}
	return nil
}

// ListDomainRedirects returns every domain_redirect row, ordered by
// domain, for the ingress controller to build redirect routes fresh
// every reconcile pass, the same "never cache, re-derive from current
// state every call" convention ListDomainBasicAuth already follows.
func (db *DB) ListDomainRedirects(ctx context.Context) ([]DomainRedirect, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, target_url, status_code FROM domain_redirect ORDER BY domain
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain redirects: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DomainRedirect
	for rows.Next() {
		var d DomainRedirect
		if err := rows.Scan(&d.Domain, &d.TargetURL, &d.StatusCode); err != nil {
			return nil, fmt.Errorf("store: scan domain redirect row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain redirect rows: %w", err)
	}
	return out, nil
}
