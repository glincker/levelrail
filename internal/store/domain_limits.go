package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DomainLimits is one row of domain_limits (migrations/3214). A zero field
// means no per-domain value: the app and instance limits apply.
type DomainLimits struct {
	Domain              string
	MaxBodyBytes        int64
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	UpdatedAt           time.Time
}

const domainLimitsColumns = `domain, max_body_bytes, read_timeout_seconds, write_timeout_seconds, updated_at`

func scanDomainLimits(row rowScanner) (DomainLimits, error) {
	var l DomainLimits
	var updated string
	if err := row.Scan(&l.Domain, &l.MaxBodyBytes, &l.ReadTimeoutSeconds, &l.WriteTimeoutSeconds, &updated); err != nil {
		return DomainLimits{}, err
	}
	if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		l.UpdatedAt = t
	}
	return l, nil
}

// GetDomainLimits returns domain's limits. found is false when none are set.
func (db *DB) GetDomainLimits(ctx context.Context, domain string) (DomainLimits, bool, error) {
	l, err := scanDomainLimits(db.QueryRowContext(ctx, `SELECT `+domainLimitsColumns+` FROM domain_limits WHERE domain = ?`, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return DomainLimits{}, false, nil
	}
	if err != nil {
		return DomainLimits{}, false, fmt.Errorf("store: get domain limits: %w", err)
	}
	return l, true, nil
}

// SetDomainLimits upserts l. Callers validate the fields first.
func (db *DB) SetDomainLimits(ctx context.Context, l DomainLimits) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_limits (`+domainLimitsColumns+`) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET
			max_body_bytes = excluded.max_body_bytes, read_timeout_seconds = excluded.read_timeout_seconds,
			write_timeout_seconds = excluded.write_timeout_seconds, updated_at = excluded.updated_at
	`, l.Domain, l.MaxBodyBytes, l.ReadTimeoutSeconds, l.WriteTimeoutSeconds, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set domain limits: %w", err)
	}
	return nil
}

// DeleteDomainLimits removes domain's limits. Idempotent.
func (db *DB) DeleteDomainLimits(ctx context.Context, domain string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM domain_limits WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: delete domain limits: %w", err)
	}
	return nil
}

// ListDomainLimits returns every row ordered by domain.
func (db *DB) ListDomainLimits(ctx context.Context) ([]DomainLimits, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+domainLimitsColumns+` FROM domain_limits ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain limits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainLimits
	for rows.Next() {
		l, err := scanDomainLimits(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan domain limits: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain limits: %w", err)
	}
	return out, nil
}
