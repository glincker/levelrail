package store

import (
	"context"
	"fmt"
)

// ListHiddenDomains returns every domain that asks search engines to stay away.
func (db *DB) ListHiddenDomains(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain FROM domain_search_visibility ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list hidden domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("store: scan hidden domain: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate hidden domains: %w", err)
	}
	return out, nil
}

// IsDomainHidden reports whether domain asks search engines to stay away.
func (db *DB) IsDomainHidden(ctx context.Context, domain string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domain_search_visibility WHERE domain = ?`, domain).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: read domain search visibility: %w", err)
	}
	return n > 0, nil
}

// SetDomainHidden hides or unhides domain from search engines. Idempotent.
func (db *DB) SetDomainHidden(ctx context.Context, domain string, hidden bool) error {
	query := `DELETE FROM domain_search_visibility WHERE domain = ?`
	if hidden {
		query = `INSERT OR IGNORE INTO domain_search_visibility (domain) VALUES (?)`
	}
	if _, err := db.ExecContext(ctx, query, domain); err != nil {
		return fmt.Errorf("store: set domain search visibility: %w", err)
	}
	return nil
}
