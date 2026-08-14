package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// domainOwner looks up domain across every table that can claim a
// hostname: service_domains (container services, migrations/0012) and
// static_site_domains (static sites, migrations/0015). Both
// claimServiceDomains (service.go) and claimStaticSiteDomains
// (static_site.go) call this before claiming a domain, so a container
// service and a static site can never end up routing the same hostname,
// the same conflict migrations/0012 closed within desired_services
// alone, now closed across both desired-state tables.
//
// Returns ("", false, nil) if domain is unclaimed anywhere.
func domainOwner(ctx context.Context, tx *sql.Tx, domain string) (owner string, found bool, err error) {
	if err := tx.QueryRowContext(ctx, `SELECT service_name FROM service_domains WHERE domain = ?`, domain).Scan(&owner); err == nil {
		return owner, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("check service_domains for %q: %w", domain, err)
	}

	if err := tx.QueryRowContext(ctx, `SELECT static_site_name FROM static_site_domains WHERE domain = ?`, domain).Scan(&owner); err == nil {
		return owner, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("check static_site_domains for %q: %w", domain, err)
	}

	return "", false, nil
}
