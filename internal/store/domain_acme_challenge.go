package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ListDNS01Domains returns every host that asked for a DNS-01 certificate.
func (db *DB) ListDNS01Domains(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain FROM domain_acme_challenge WHERE challenge = 'dns01' ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list dns01 domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("store: scan dns01 domain: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate dns01 domains: %w", err)
	}
	return out, nil
}

// DomainDNS01 reports whether domain uses DNS-01 and since when.
func (db *DB) DomainDNS01(ctx context.Context, domain string) (bool, time.Time, error) {
	var at string
	err := db.QueryRowContext(ctx, `SELECT requested_at FROM domain_acme_challenge WHERE domain = ?`, domain).Scan(&at)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, fmt.Errorf("store: read domain dns01: %w", err)
	}
	t, _ := time.Parse(time.RFC3339, at)
	return true, t, nil
}

// SetDomainDNS01 switches one host to DNS-01 or back to the default. Idempotent.
func (db *DB) SetDomainDNS01(ctx context.Context, domain string, on bool) error {
	var err error
	if on {
		_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO domain_acme_challenge (domain, challenge, requested_at) VALUES (?, 'dns01', ?)`,
			domain, time.Now().UTC().Format(time.RFC3339))
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM domain_acme_challenge WHERE domain = ?`, domain)
	}
	if err != nil {
		return fmt.Errorf("store: set domain dns01: %w", err)
	}
	return nil
}
