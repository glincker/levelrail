package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// EffectiveServiceDomains is the host set an app routes while it is tagged
// with envID: that environment's own set when non-empty, else the app's
// default set.
func EffectiveServiceDomains(base []string, envID string, sets map[string][]string) []string {
	if envID != "" {
		if own := sets[envID]; len(own) > 0 {
			return own
		}
	}
	return base
}

// DomainRewrite rewrites hostnames when a domain set is copied to another
// environment. Prefix is prepended; otherwise a hostname equal to or under
// Find has that suffix replaced by Replace. Non-matching hosts are unchanged.
type DomainRewrite struct {
	Prefix  string `json:"prefix,omitempty"`
	Find    string `json:"find,omitempty"`
	Replace string `json:"replace,omitempty"`
}

// Empty reports whether the rewrite changes nothing.
func (r DomainRewrite) Empty() bool { return r.Prefix == "" && r.Find == "" }

// Apply returns host after the rewrite.
func (r DomainRewrite) Apply(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch {
	case r.Prefix != "":
		return strings.ToLower(r.Prefix) + host
	case r.Find != "":
		find := strings.ToLower(r.Find)
		repl := strings.ToLower(r.Replace)
		if host == find {
			return repl
		}
		if strings.HasSuffix(host, "."+find) {
			return strings.TrimSuffix(host, find) + repl
		}
	}
	return host
}

// ApplyAll rewrites every host, dropping duplicates and blanks.
func (r DomainRewrite) ApplyAll(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if n := r.Apply(h); n != "" && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// ListServiceEnvironmentDomains returns name's per-environment domain sets,
// keyed by environment ID.
func (db *DB) ListServiceEnvironmentDomains(ctx context.Context, name string) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT environment_id, domain FROM service_environment_domains
		WHERE service_name = ? ORDER BY environment_id, domain`, name)
	if err != nil {
		return nil, fmt.Errorf("store: list environment domains for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]string{}
	for rows.Next() {
		var env, domain string
		if err := rows.Scan(&env, &domain); err != nil {
			return nil, fmt.Errorf("store: list environment domains for %q: scan: %w", name, err)
		}
		out[env] = append(out[env], domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list environment domains for %q: %w", name, err)
	}
	return out, nil
}

// ListAllServiceEnvironmentDomains returns every app's per-environment
// domain sets, keyed by app name then environment ID.
func (db *DB) ListAllServiceEnvironmentDomains(ctx context.Context) (map[string]map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, environment_id, domain FROM service_environment_domains
		ORDER BY service_name, environment_id, domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list all environment domains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string][]string{}
	for rows.Next() {
		var svc, env, domain string
		if err := rows.Scan(&svc, &env, &domain); err != nil {
			return nil, fmt.Errorf("store: list all environment domains: scan: %w", err)
		}
		if out[svc] == nil {
			out[svc] = map[string][]string{}
		}
		out[svc][env] = append(out[svc][env], domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list all environment domains: %w", err)
	}
	return out, nil
}

// EditServiceEnvironmentDomains rewrites only name's domain set for envID in
// one transaction, claiming the new hostnames (ErrDomainTaken via errors.As).
// Returns ErrServiceNotFound or ErrEnvironmentNotFound.
func (db *DB) EditServiceEnvironmentDomains(ctx context.Context, name, envID string, edit func(current []string) ([]string, error)) (domains []string, changed bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("store: edit environment domains for %q: begin transaction: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT domains FROM desired_services WHERE name = ?`, name).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrServiceNotFound
	} else if err != nil {
		return nil, false, fmt.Errorf("store: edit environment domains for %q: read app: %w", name, err)
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM environments WHERE id = ?`, envID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrEnvironmentNotFound
	} else if err != nil {
		return nil, false, fmt.Errorf("store: edit environment domains for %q: read environment: %w", name, err)
	}

	current, err := envDomainsTx(ctx, tx, name, envID)
	if err != nil {
		return nil, false, err
	}
	next, err := edit(slices.Clone(current))
	if err != nil {
		return nil, false, err
	}
	next = nonNilSlice(next)
	if slices.Equal(current, next) {
		return next, false, nil
	}
	if err := replaceEnvDomainsTx(ctx, tx, name, envID, next); err != nil {
		return nil, false, err
	}
	var base []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &base); err != nil {
			return nil, false, fmt.Errorf("store: edit environment domains for %q: decode: %w", name, err)
		}
	}
	if err := claimServiceDomains(ctx, tx, name, base); err != nil {
		return nil, false, fmt.Errorf("store: edit environment domains for %q: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("store: edit environment domains for %q: commit: %w", name, err)
	}
	return next, true, nil
}

func envDomainsTx(ctx context.Context, tx *sql.Tx, name, envID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT domain FROM service_environment_domains
		WHERE service_name = ? AND environment_id = ? ORDER BY domain`, name, envID)
	if err != nil {
		return nil, fmt.Errorf("store: read environment domains for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("store: read environment domains for %q: scan: %w", name, err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func replaceEnvDomainsTx(ctx context.Context, tx *sql.Tx, name, envID string, domains []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_environment_domains WHERE service_name = ? AND environment_id = ?`, name, envID); err != nil {
		return fmt.Errorf("store: clear environment domains for %q: %w", name, err)
	}
	for _, d := range domains {
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_environment_domains (service_name, environment_id, domain) VALUES (?, ?, ?)`, name, envID, d); err != nil {
			return fmt.Errorf("store: write environment domain %q for %q: %w", d, name, err)
		}
	}
	return nil
}

// heldEnvDomains returns every domain name holds in any environment set.
func heldEnvDomains(ctx context.Context, tx *sql.Tx, name string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT domain FROM service_environment_domains WHERE service_name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("list environment domain claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan environment domain claim: %w", err)
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out, rows.Err()
}
