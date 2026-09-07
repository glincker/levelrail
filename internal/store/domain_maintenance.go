package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetDomainMaintenance reports whether domain currently has maintenance
// mode enabled. false is the default, valid state for any domain, not
// an error.
func (db *DB) GetDomainMaintenance(ctx context.Context, domain string) (bool, error) {
	var d string
	err := db.QueryRowContext(ctx, `SELECT domain FROM domain_maintenance WHERE domain = ?`, domain).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: get domain maintenance: %w", err)
	}
	return true, nil
}

// SetDomainMaintenance enables maintenance mode for domain. Idempotent:
// setting it on a domain that already has it enabled is not an error.
func (db *DB) SetDomainMaintenance(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO domain_maintenance (domain) VALUES (?) ON CONFLICT (domain) DO NOTHING`, domain)
	if err != nil {
		return fmt.Errorf("store: set domain maintenance: %w", err)
	}
	return nil
}

// DeleteDomainMaintenance disables maintenance mode for domain, if
// enabled. Idempotent: clearing a domain with none configured is not an
// error.
func (db *DB) DeleteDomainMaintenance(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_maintenance WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain maintenance: %w", err)
	}
	return nil
}

// ListDomainMaintenance returns every domain currently in maintenance
// mode, ordered by domain, for the ingress controller to check fresh
// every reconcile pass, the same "never cache, re-derive from current
// state" convention ListDomainBasicAuth already follows.
func (db *DB) ListDomainMaintenance(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain FROM domain_maintenance ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain maintenance: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("store: scan domain maintenance row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain maintenance rows: %w", err)
	}
	return out, nil
}
