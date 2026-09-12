package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Domain WAF modes (domain_waf.waf_mode, migrations/0093). DomainWAFModeDetect
// is the default: OWASP CRS runs but only logs matches, never rejects a
// request, matching Coraza's own documented SecRuleEngine DetectionOnly
// behavior.
const (
	DomainWAFModeDetect = "detect"
	DomainWAFModeBlock  = "block"
)

// DomainWAF is one row of the domain_waf table: opt-in WAF and rate
// limiting for a domain already present in service_domains. RateLimitRPS
// of 0 means rate limiting is off, independent of WAFEnabled.
type DomainWAF struct {
	Domain         string
	WAFEnabled     bool
	WAFMode        string
	RateLimitRPS   int
	RateLimitBurst int
}

// GetDomainWAF returns domain's current WAF/rate-limit configuration.
// found is false when the domain has no row, the default, valid state
// for any domain, not an error; the returned DomainWAF is the zero
// value plus DomainWAFModeDetect in that case, mirroring what a fresh
// row would look like before an operator changes anything.
func (db *DB) GetDomainWAF(ctx context.Context, domain string) (DomainWAF, bool, error) {
	var d DomainWAF
	var wafEnabled int
	err := db.QueryRowContext(ctx, `
		SELECT domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst
		FROM domain_waf WHERE domain = ?
	`, domain).Scan(&d.Domain, &wafEnabled, &d.WAFMode, &d.RateLimitRPS, &d.RateLimitBurst)
	if errors.Is(err, sql.ErrNoRows) {
		return DomainWAF{Domain: domain, WAFMode: DomainWAFModeDetect}, false, nil
	}
	if err != nil {
		return DomainWAF{}, false, fmt.Errorf("store: get domain waf: %w", err)
	}
	d.WAFEnabled = wafEnabled != 0
	return d, true, nil
}

// SetDomainWAF upserts domain's WAF/rate-limit configuration. mode must
// already be validated by the caller (internal/api); this method
// performs no validation of its own, the same division of
// responsibility store.SetDomainBasicAuth already draws with its own
// caller.
func (db *DB) SetDomainWAF(ctx context.Context, domain string, wafEnabled bool, mode string, rateLimitRPS, rateLimitBurst int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_waf (domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET
			waf_enabled = excluded.waf_enabled,
			waf_mode = excluded.waf_mode,
			rate_limit_rps = excluded.rate_limit_rps,
			rate_limit_burst = excluded.rate_limit_burst
	`, domain, boolToInt(wafEnabled), mode, rateLimitRPS, rateLimitBurst)
	if err != nil {
		return fmt.Errorf("store: set domain waf: %w", err)
	}
	return nil
}

// DeleteDomainWAF removes domain's WAF/rate-limit row, if any, resetting
// it to the default (WAF off, rate limiting off). Idempotent: deleting a
// domain with none configured is not an error.
func (db *DB) DeleteDomainWAF(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_waf WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain waf: %w", err)
	}
	return nil
}

// ListDomainWAF returns every domain_waf row, ordered by domain, for the
// ingress controller to build WAF/rate-limit handlers fresh every
// reconcile pass, the same "never cache, re-derive from current state
// every call" convention ListDomainBasicAuth already follows.
func (db *DB) ListDomainWAF(ctx context.Context) ([]DomainWAF, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst
		FROM domain_waf ORDER BY domain
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain waf: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DomainWAF
	for rows.Next() {
		var d DomainWAF
		var wafEnabled int
		if err := rows.Scan(&d.Domain, &wafEnabled, &d.WAFMode, &d.RateLimitRPS, &d.RateLimitBurst); err != nil {
			return nil, fmt.Errorf("store: scan domain waf row: %w", err)
		}
		d.WAFEnabled = wafEnabled != 0
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain waf rows: %w", err)
	}
	return out, nil
}
