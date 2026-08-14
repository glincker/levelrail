package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// StaticSite is what internal/reconcile/ingress's controller reads to
// point Caddy's file_server handler directly at a directory on disk, no
// container involved. See migrations/0015_static_sites.sql for why this
// is a separate table from DesiredService rather than a variant of it:
// build.type: static (CLAUDE.md 4.4) has no image, no port, and no
// running container to converge to, so it never belongs in
// desired_services at all.
type StaticSite struct {
	Name    string
	Domains []string
	// RootDir is the local filesystem directory internal/deploy copied
	// this site's built static files into (under the control plane's
	// data directory, never the ephemeral git checkout dir itself, which
	// a caller like internal/webhook removes as soon as the deploy call
	// returns). The ingress controller passes this straight to Caddy's
	// file_server handler as its root.
	RootDir string
}

// SaveStaticSite creates or fully replaces a static site's desired
// state, the same "whole record, one write" shape SaveDesiredService
// uses for container services: internal/deploy.Pipeline always produces
// a complete StaticSite from one app.yaml service block, never a partial
// update.
//
// The whole write, including reconciling site.Domains against
// static_site_domains, happens in one transaction: either the site's
// state and its domain claims both land, or neither does. Returns
// *ErrDomainTaken (via errors.As) if any domain in site.Domains is
// already claimed by a different static site or by a container service
// (domainOwner checks both service_domains and static_site_domains);
// the caller's entire write is rejected in that case, matching
// SaveDesiredService's own domain-conflict behavior.
func (db *DB) SaveStaticSite(ctx context.Context, site StaticSite) error {
	domainsJSON, err := json.Marshal(nonNilSlice(site.Domains))
	if err != nil {
		return fmt.Errorf("store: marshal domains for static site %q: %w", site.Name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save static site %q: begin transaction: %w", site.Name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO static_sites (name, domains, root_dir, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			domains    = excluded.domains,
			root_dir   = excluded.root_dir,
			updated_at = excluded.updated_at
	`, site.Name, string(domainsJSON), site.RootDir)
	if err != nil {
		return fmt.Errorf("store: save static site %q: %w", site.Name, err)
	}

	if err := claimStaticSiteDomains(ctx, tx, site.Name, site.Domains); err != nil {
		return fmt.Errorf("store: save static site %q: %w", site.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save static site %q: commit: %w", site.Name, err)
	}
	return nil
}

// claimStaticSiteDomains is claimServiceDomains's (service.go)
// counterpart for static_site_domains: same replace-the-whole-set
// shape, same shared domainOwner conflict check across both domain
// tables (migrations/0015).
func claimStaticSiteDomains(ctx context.Context, tx *sql.Tx, siteName string, domains []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM static_site_domains WHERE static_site_name = ?`, siteName); err != nil {
		return fmt.Errorf("clear existing domain claims: %w", err)
	}

	claimed := make(map[string]bool, len(domains))
	for _, domain := range domains {
		if domain == "" || claimed[domain] {
			continue
		}
		claimed[domain] = true

		owner, found, err := domainOwner(ctx, tx, domain)
		if err != nil {
			return fmt.Errorf("check domain %q availability: %w", domain, err)
		}
		if found {
			return &ErrDomainTaken{Domain: domain, Owner: owner}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO static_site_domains (domain, static_site_name) VALUES (?, ?)
		`, domain, siteName); err != nil {
			return fmt.Errorf("claim domain %q: %w", domain, err)
		}
	}
	return nil
}

// ListStaticSites returns every saved static site, ordered by name. The
// ingress controller (internal/reconcile/ingress) calls this on every
// reconcile pass, the same "read fresh, never cached" treatment
// ListDesiredServices already gets.
func (db *DB) ListStaticSites(ctx context.Context) ([]StaticSite, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, domains, root_dir
		FROM static_sites
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list static sites: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []StaticSite
	for rows.Next() {
		var (
			site        StaticSite
			domainsJSON string
		)
		if err := rows.Scan(&site.Name, &domainsJSON, &site.RootDir); err != nil {
			return nil, fmt.Errorf("store: scan static site row: %w", err)
		}
		if err := json.Unmarshal([]byte(domainsJSON), &site.Domains); err != nil {
			return nil, fmt.Errorf("store: unmarshal domains for static site %q: %w", site.Name, err)
		}
		out = append(out, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate static site rows: %w", err)
	}
	return out, nil
}
