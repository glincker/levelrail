package store

import (
	"context"
	"fmt"
	"strings"
)

// GatedDomains returns the subset of domains that currently sit behind a
// basic auth or an enabled forward-auth gate, in one query so a previews
// list does not issue one lookup per row. Only domain names leave the store.
func (db *DB) GatedDomains(ctx context.Context, domains []string) (map[string]bool, error) {
	out := make(map[string]bool, len(domains))
	if len(domains) == 0 {
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(domains)), ",")
	args := make([]any, 0, len(domains)*2)
	for range 2 {
		for _, d := range domains {
			args = append(args, d)
		}
	}
	rows, err := db.QueryContext(ctx, `
		SELECT domain FROM domain_basic_auth WHERE domain IN (`+marks+`)
		UNION
		SELECT domain FROM domain_auth_gate WHERE enabled = 1 AND domain IN (`+marks+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: gated domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("store: scan gated domain: %w", err)
		}
		out[d] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate gated domains: %w", err)
	}
	return out, nil
}
