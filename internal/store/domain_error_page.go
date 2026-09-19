package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Allowed domain_error_page.status_code values (migrations/0105): the
// common failure statuses an operator would actually want a custom page
// for, not a fully generic arbitrary-status-code system.
const (
	DomainErrorPageNotFound           = 404
	DomainErrorPageServerError        = 500
	DomainErrorPageBadGateway         = 502
	DomainErrorPageServiceUnavailable = 503
)

// DomainErrorPage is one row of the domain_error_page table: a
// status-code-to-HTML-body mapping for a domain already present in
// service_domains.
type DomainErrorPage struct {
	Domain     string
	StatusCode int
	Body       string
}

// ListDomainErrorPages returns every custom error page configured for
// domain, ordered by status code.
func (db *DB) ListDomainErrorPages(ctx context.Context, domain string) ([]DomainErrorPage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, status_code, body FROM domain_error_page WHERE domain = ? ORDER BY status_code
	`, domain)
	if err != nil {
		return nil, fmt.Errorf("store: list domain error pages: %w", err)
	}
	return scanDomainErrorPages(rows)
}

// ListAllDomainErrorPages returns every domain_error_page row across
// every domain, ordered by domain then status code, for the ingress
// controller to build error-page routes fresh every reconcile pass, the
// same "never cache, re-derive from current state every call"
// convention ListDomainRedirects already follows.
func (db *DB) ListAllDomainErrorPages(ctx context.Context) ([]DomainErrorPage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, status_code, body FROM domain_error_page ORDER BY domain, status_code
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list all domain error pages: %w", err)
	}
	return scanDomainErrorPages(rows)
}

// SetDomainErrorPage upserts one status-code-to-body mapping for domain.
// statusCode and body must already be validated by the caller
// (internal/api); this method performs no validation of its own, the
// same division of responsibility store.SetDomainRedirect draws.
func (db *DB) SetDomainErrorPage(ctx context.Context, domain string, statusCode int, body string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_error_page (domain, status_code, body)
		VALUES (?, ?, ?)
		ON CONFLICT (domain, status_code) DO UPDATE SET body = excluded.body
	`, domain, statusCode, body)
	if err != nil {
		return fmt.Errorf("store: set domain error page: %w", err)
	}
	return nil
}

// DeleteDomainErrorPage removes domain's mapping for statusCode, if any.
// Idempotent: deleting one that doesn't exist is not an error.
func (db *DB) DeleteDomainErrorPage(ctx context.Context, domain string, statusCode int) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_error_page WHERE domain = ? AND status_code = ?`, domain, statusCode)
	if err != nil {
		return fmt.Errorf("store: delete domain error page: %w", err)
	}
	return nil
}

// DeleteDomainErrorPages removes every custom error page configured for
// domain. Idempotent: clearing a domain with none configured is not an
// error.
func (db *DB) DeleteDomainErrorPages(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_error_page WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain error pages: %w", err)
	}
	return nil
}

func scanDomainErrorPages(rows *sql.Rows) ([]DomainErrorPage, error) {
	defer func() {
		_ = rows.Close()
	}()

	var out []DomainErrorPage
	for rows.Next() {
		var p DomainErrorPage
		if err := rows.Scan(&p.Domain, &p.StatusCode, &p.Body); err != nil {
			return nil, fmt.Errorf("store: scan domain error page row: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain error page rows: %w", err)
	}
	return out, nil
}
