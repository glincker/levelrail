package store

import (
	"context"
	"database/sql"
	"fmt"
)

// IngressSettings is the platform-wide ingress configuration row
// (migrations/0024_ingress_settings.sql): whether to obtain real ACME
// certificates instead of Caddy's offline internal issuer, and an
// optional primary domain for the control plane's own dashboard. There
// is always exactly one of these (id = 1), never zero and never more
// than one, so GetIngressSettings never returns a not-found error the
// way GetProject/GetDesiredService do: the row is seeded by the
// migration itself.
type IngressSettings struct {
	// PrimaryDomain is the control plane dashboard's own hostname, if an
	// operator has set one. Empty string means "not set", the same
	// "absence is real, permanent state" convention
	// DesiredService.ProjectID's own doc comment already establishes for
	// a nullable text column.
	PrimaryDomain string
	// ACMEEnabled switches every currently-routed host from Caddy's
	// offline internal issuer to real ACME. False (the default a fresh
	// migration seeds) is byte-identical to this codebase's behavior
	// before this table existed: see internal/ingress/routes.go's
	// BuildRoutesConfig for where this flag actually changes anything.
	ACMEEnabled bool
	// ACMEEmail is the ACME account contact address. Required by
	// internal/api's PUT /api/v1/settings/ingress validation whenever
	// ACMEEnabled is true; may be empty while ACMEEnabled is false.
	ACMEEmail string
	// ACMEDirectoryURL overrides the ACME CA's directory endpoint. Empty
	// means internal/ingress.NewACMEIssuer leaves Caddy's own compiled-in
	// default in place (Let's Encrypt's real production directory). See
	// this migration's own comment for why an operator would point this
	// at Let's Encrypt's staging directory instead.
	ACMEDirectoryURL string
}

// GetIngressSettings returns the single ingress_settings row. Always
// succeeds against a migrated database: the migration itself inserts the
// row (id = 1), so there is no "not found" case for callers to handle,
// unlike every other Get* accessor in this package.
func (db *DB) GetIngressSettings(ctx context.Context) (IngressSettings, error) {
	var (
		s                IngressSettings
		primaryDomain    sql.NullString
		acmeEnabled      int
		acmeEmail        sql.NullString
		acmeDirectoryURL sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT primary_domain, acme_enabled, acme_email, acme_directory_url
		FROM ingress_settings
		WHERE id = 1
	`).Scan(&primaryDomain, &acmeEnabled, &acmeEmail, &acmeDirectoryURL)
	if err != nil {
		return IngressSettings{}, fmt.Errorf("store: get ingress settings: %w", err)
	}

	s.PrimaryDomain = primaryDomain.String
	s.ACMEEnabled = acmeEnabled != 0
	s.ACMEEmail = acmeEmail.String
	s.ACMEDirectoryURL = acmeDirectoryURL.String
	return s, nil
}

// UpdateIngressSettings replaces the single ingress_settings row in
// full, the same "no partial update, the whole record is always written
// as a whole" convention SaveDesiredService already establishes for
// desired_services: a settings form always submits its complete current
// state, there's no legitimate "patch just one field" caller for a
// four-column singleton row. internal/api's PUT /api/v1/settings/ingress
// is expected to have already validated s (acme_email required and
// well-formed whenever s.ACMEEnabled is true) before calling this;
// this method itself performs no validation, matching the "store trusts
// its caller to have validated business rules, and only enforces
// what the schema itself can" division of responsibility
// SaveDesiredService's own domain-uniqueness enforcement is the
// exception to, not the rule.
func (db *DB) UpdateIngressSettings(ctx context.Context, s IngressSettings) error {
	acmeEnabled := 0
	if s.ACMEEnabled {
		acmeEnabled = 1
	}

	_, err := db.ExecContext(ctx, `
		UPDATE ingress_settings
		SET primary_domain = ?, acme_enabled = ?, acme_email = ?, acme_directory_url = ?
		WHERE id = 1
	`,
		sql.NullString{String: s.PrimaryDomain, Valid: s.PrimaryDomain != ""},
		acmeEnabled,
		sql.NullString{String: s.ACMEEmail, Valid: s.ACMEEmail != ""},
		sql.NullString{String: s.ACMEDirectoryURL, Valid: s.ACMEDirectoryURL != ""},
	)
	if err != nil {
		return fmt.Errorf("store: update ingress settings: %w", err)
	}
	return nil
}

// ServiceDomain is one row of the service_domains table
// (migrations/0012_service_domains.sql): a domain currently claimed by a
// named service. Read-only projection for GET /api/v1/domains
// (internal/api), the centralized cross-app domain list; this is not a
// new tracking mechanism, service_domains is already kept in sync by
// SaveDesiredService on every write (see that method's own doc comment).
type ServiceDomain struct {
	Domain      string
	ServiceName string
}

// ListServiceDomains returns every row in service_domains, ordered by
// domain. This is the same table domainOwner (domains.go) already
// queries for uniqueness enforcement, exposed here as a plain listing
// for GET /api/v1/domains rather than a single-domain lookup.
func (db *DB) ListServiceDomains(ctx context.Context) ([]ServiceDomain, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, service_name
		FROM service_domains
		ORDER BY domain
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list service domains: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ServiceDomain
	for rows.Next() {
		var d ServiceDomain
		if err := rows.Scan(&d.Domain, &d.ServiceName); err != nil {
			return nil, fmt.Errorf("store: scan service domain row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate service domain rows: %w", err)
	}
	return out, nil
}
