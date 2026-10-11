package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DomainTrafficPolicy is one row of domain_traffic_policies (migrations/0421):
// the JSON spec of one traffic control kind for one domain. The caller
// (internal/api) validates Spec with internal/trafficpolicy before saving.
type DomainTrafficPolicy struct {
	Domain    string
	Kind      string
	Spec      []byte
	UpdatedAt string
}

// GetDomainTrafficPolicy returns domain's spec for kind; found is false when
// the domain uses the default behavior.
func (db *DB) GetDomainTrafficPolicy(ctx context.Context, domain, kind string) (DomainTrafficPolicy, bool, error) {
	p := DomainTrafficPolicy{Domain: domain, Kind: kind}
	var spec string
	err := db.QueryRowContext(ctx, `SELECT spec, updated_at FROM domain_traffic_policies WHERE domain = ? AND kind = ?`, domain, kind).Scan(&spec, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return DomainTrafficPolicy{}, false, fmt.Errorf("store: get domain traffic policy %s/%s: %w", domain, kind, err)
	}
	p.Spec = []byte(spec)
	return p, true, nil
}

// ListDomainTrafficPoliciesFor returns every kind configured for domain.
func (db *DB) ListDomainTrafficPoliciesFor(ctx context.Context, domain string) ([]DomainTrafficPolicy, error) {
	return db.listDomainTrafficPolicies(ctx, `WHERE domain = ?`, domain)
}

// ListDomainTrafficPolicies returns every row, read by the ingress
// reconciler on each pass.
func (db *DB) ListDomainTrafficPolicies(ctx context.Context) ([]DomainTrafficPolicy, error) {
	return db.listDomainTrafficPolicies(ctx, ``)
}

func (db *DB) listDomainTrafficPolicies(ctx context.Context, where string, args ...any) ([]DomainTrafficPolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, kind, spec, updated_at FROM domain_traffic_policies `+where+` ORDER BY domain, kind`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list domain traffic policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainTrafficPolicy
	for rows.Next() {
		var p DomainTrafficPolicy
		var spec string
		if err := rows.Scan(&p.Domain, &p.Kind, &spec, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan domain traffic policy: %w", err)
		}
		p.Spec = []byte(spec)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain traffic policies: %w", err)
	}
	return out, nil
}

// SetDomainTrafficPolicy upserts domain's spec for kind.
func (db *DB) SetDomainTrafficPolicy(ctx context.Context, domain, kind string, spec []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_traffic_policies (domain, kind, spec, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (domain, kind) DO UPDATE SET spec = excluded.spec, updated_at = excluded.updated_at
	`, domain, kind, string(spec), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set domain traffic policy %s/%s: %w", domain, kind, err)
	}
	return nil
}

// DeleteDomainTrafficPolicy restores the default for kind. Idempotent.
func (db *DB) DeleteDomainTrafficPolicy(ctx context.Context, domain, kind string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM domain_traffic_policies WHERE domain = ? AND kind = ?`, domain, kind); err != nil {
		return fmt.Errorf("store: delete domain traffic policy %s/%s: %w", domain, kind, err)
	}
	return nil
}
