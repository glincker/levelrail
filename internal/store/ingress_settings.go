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
	// HSTSEnabled sends Strict-Transport-Security for the dashboard's own
	// responses (migrations/0254_ingress_settings_hsts.sql). False (the
	// default) matches api.Router.hstsEnabled's existing default: see
	// that field's doc comment for why this stays opt-in. The
	// APP_ENABLE_HSTS env var still works and takes precedence when set,
	// so existing deployments that already export it keep behaving
	// exactly as before (see cmd/levelrail/main.go's hstsEnabled).
	HSTSEnabled bool
	// FallbackDomainsDisabled turns off the automatic
	// <app>.<dashed-ip>.sslip.io hostname apps without a domain get
	// (migrations/0290). Default false: the hostname is on.
	FallbackDomainsDisabled bool
	// PublicHTTPSPort is the port clients use to reach this ingress when a
	// proxy fronts it (migrations/0416). 0 means the ingress listen port.
	PublicHTTPSPort int
	// TLSTerminatedUpstream marks a proxy in front as the TLS owner: no ACME
	// here and links use the public port (migrations/0416).
	TLSTerminatedUpstream bool
	// AppsBaseDomain is the suffix apps created without a domain get as
	// <app>.<base> (migrations/0418). Empty means not set.
	AppsBaseDomain string
	// DNSCNAMETarget makes automatic records CNAMEs to this host instead of
	// A/AAAA to the detected public IP. DNSTTLSeconds 0 is the provider's auto.
	DNSCNAMETarget string
	DNSTTLSeconds  int
	// DNSProxied turns Cloudflare's proxy on for records Levelrail creates.
	DNSProxied bool
}

// MaxDNSTTLSeconds is the highest valid DNSTTLSeconds (one day).
const MaxDNSTTLSeconds = 86400

// MaxPublicHTTPSPort is the highest valid PublicHTTPSPort.
const MaxPublicHTTPSPort = 65535

// ValidatePublicHTTPSPort rejects ports outside 0..MaxPublicHTTPSPort.
func ValidatePublicHTTPSPort(p int) error {
	if p < 0 || p > MaxPublicHTTPSPort {
		return fmt.Errorf("store: public_https_port %d is out of range 0-%d", p, MaxPublicHTTPSPort)
	}
	return nil
}

// EffectiveACMEEnabled is false whenever TLS is terminated upstream, so a
// stored acme_enabled is kept but never acted on.
func (s IngressSettings) EffectiveACMEEnabled() bool {
	return s.ACMEEnabled && !s.TLSTerminatedUpstream
}

// PublicLinkPort is the explicit port generated links must carry: the
// configured public port, 443 when TLS is upstream without one, else 0
// (the caller's own default applies).
func (s IngressSettings) PublicLinkPort() int {
	switch {
	case s.PublicHTTPSPort != 0:
		return s.PublicHTTPSPort
	case s.TLSTerminatedUpstream:
		return 443
	default:
		return 0
	}
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
		hstsEnabled      int
		fallbackDisabled int
		publicHTTPSPort  int
		tlsUpstream      int
		appsBaseDomain   sql.NullString
		cnameTarget      sql.NullString
		dnsTTL           int
		dnsProxied       int
	)
	err := db.QueryRowContext(ctx, `
		SELECT primary_domain, acme_enabled, acme_email, acme_directory_url, hsts_enabled, fallback_domains_disabled, public_https_port, tls_terminated_upstream,
			apps_base_domain, dns_cname_target, dns_ttl_seconds, dns_proxied
		FROM ingress_settings
		WHERE id = 1
	`).Scan(&primaryDomain, &acmeEnabled, &acmeEmail, &acmeDirectoryURL, &hstsEnabled, &fallbackDisabled, &publicHTTPSPort, &tlsUpstream,
		&appsBaseDomain, &cnameTarget, &dnsTTL, &dnsProxied)
	if err != nil {
		return IngressSettings{}, fmt.Errorf("store: get ingress settings: %w", err)
	}

	s.PrimaryDomain = primaryDomain.String
	s.ACMEEnabled = acmeEnabled != 0
	s.ACMEEmail = acmeEmail.String
	s.ACMEDirectoryURL = acmeDirectoryURL.String
	s.HSTSEnabled = hstsEnabled != 0
	s.FallbackDomainsDisabled = fallbackDisabled != 0
	s.PublicHTTPSPort = publicHTTPSPort
	s.TLSTerminatedUpstream = tlsUpstream != 0
	s.AppsBaseDomain = appsBaseDomain.String
	s.DNSCNAMETarget = cnameTarget.String
	s.DNSTTLSeconds = dnsTTL
	s.DNSProxied = dnsProxied != 0
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
	if err := ValidatePublicHTTPSPort(s.PublicHTTPSPort); err != nil {
		return err
	}
	if s.DNSTTLSeconds < 0 || s.DNSTTLSeconds > MaxDNSTTLSeconds {
		return fmt.Errorf("store: dns_ttl_seconds %d is out of range 0-%d", s.DNSTTLSeconds, MaxDNSTTLSeconds)
	}
	dnsProxied := 0
	if s.DNSProxied {
		dnsProxied = 1
	}
	tlsUpstream := 0
	if s.TLSTerminatedUpstream {
		tlsUpstream = 1
	}
	acmeEnabled := 0
	if s.ACMEEnabled {
		acmeEnabled = 1
	}
	hstsEnabled := 0
	if s.HSTSEnabled {
		hstsEnabled = 1
	}

	fallbackDisabled := 0
	if s.FallbackDomainsDisabled {
		fallbackDisabled = 1
	}

	_, err := db.ExecContext(ctx, `
		UPDATE ingress_settings
		SET primary_domain = ?, acme_enabled = ?, acme_email = ?, acme_directory_url = ?, hsts_enabled = ?, fallback_domains_disabled = ?, public_https_port = ?, tls_terminated_upstream = ?,
			apps_base_domain = ?, dns_cname_target = ?, dns_ttl_seconds = ?, dns_proxied = ?
		WHERE id = 1
	`,
		sql.NullString{String: s.PrimaryDomain, Valid: s.PrimaryDomain != ""},
		acmeEnabled,
		sql.NullString{String: s.ACMEEmail, Valid: s.ACMEEmail != ""},
		sql.NullString{String: s.ACMEDirectoryURL, Valid: s.ACMEDirectoryURL != ""},
		hstsEnabled,
		fallbackDisabled,
		s.PublicHTTPSPort,
		tlsUpstream,
		sql.NullString{String: s.AppsBaseDomain, Valid: s.AppsBaseDomain != ""},
		sql.NullString{String: s.DNSCNAMETarget, Valid: s.DNSCNAMETarget != ""},
		s.DNSTTLSeconds,
		dnsProxied,
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
