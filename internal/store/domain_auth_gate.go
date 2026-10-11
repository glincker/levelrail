package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DomainAuthGate is one row of domain_auth_gate (migrations/0510): a
// forward-auth check run by the ingress before a domain's backend. The auth
// URL is not secret. TimeoutSeconds 0 means the APP_AUTH_GATE_TIMEOUT default.
type DomainAuthGate struct {
	Domain         string
	Enabled        bool
	AuthURL        string
	CheckPath      string
	CopyHeaders    []string
	TimeoutSeconds int
	UpdatedAt      time.Time
}

const domainAuthGateColumns = `domain, enabled, auth_url, check_path, copy_headers, timeout_seconds, updated_at`

func scanDomainAuthGate(row rowScanner) (DomainAuthGate, error) {
	var g DomainAuthGate
	var enabled int
	var headers, updated string
	if err := row.Scan(&g.Domain, &enabled, &g.AuthURL, &g.CheckPath, &headers, &g.TimeoutSeconds, &updated); err != nil {
		return DomainAuthGate{}, err
	}
	g.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(headers), &g.CopyHeaders); err != nil {
		return DomainAuthGate{}, fmt.Errorf("decode copy_headers for %s: %w", g.Domain, err)
	}
	if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		g.UpdatedAt = t
	}
	return g, nil
}

// GetDomainAuthGate returns domain's auth gate. found is false when none is
// configured, the default state for every domain.
func (db *DB) GetDomainAuthGate(ctx context.Context, domain string) (DomainAuthGate, bool, error) {
	g, err := scanDomainAuthGate(db.QueryRowContext(ctx, `SELECT `+domainAuthGateColumns+` FROM domain_auth_gate WHERE domain = ?`, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return DomainAuthGate{}, false, nil
	}
	if err != nil {
		return DomainAuthGate{}, false, fmt.Errorf("store: get domain auth gate: %w", err)
	}
	return g, true, nil
}

// SetDomainAuthGate upserts g. Callers validate the fields first.
func (db *DB) SetDomainAuthGate(ctx context.Context, g DomainAuthGate) error {
	headers := g.CopyHeaders
	if headers == nil {
		headers = []string{}
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("store: encode domain auth gate headers: %w", err)
	}
	enabled := 0
	if g.Enabled {
		enabled = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO domain_auth_gate (`+domainAuthGateColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET
			enabled = excluded.enabled, auth_url = excluded.auth_url, check_path = excluded.check_path,
			copy_headers = excluded.copy_headers, timeout_seconds = excluded.timeout_seconds, updated_at = excluded.updated_at
	`, g.Domain, enabled, g.AuthURL, g.CheckPath, string(encoded), g.TimeoutSeconds, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set domain auth gate: %w", err)
	}
	return nil
}

// DeleteDomainAuthGate removes domain's auth gate. Idempotent.
func (db *DB) DeleteDomainAuthGate(ctx context.Context, domain string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM domain_auth_gate WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: delete domain auth gate: %w", err)
	}
	return nil
}

// ListDomainAuthGates returns every auth gate row ordered by domain.
func (db *DB) ListDomainAuthGates(ctx context.Context) ([]DomainAuthGate, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+domainAuthGateColumns+` FROM domain_auth_gate ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain auth gates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DomainAuthGate
	for rows.Next() {
		g, err := scanDomainAuthGate(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan domain auth gate: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain auth gates: %w", err)
	}
	return out, nil
}
