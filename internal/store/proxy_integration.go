package store

import (
	"context"
	"fmt"
	"time"
)

// Proxy integration modes (migrations/0417).
const (
	ProxyIntegrationOff         = "off"
	ProxyIntegrationTraefikFile = "traefik_file"
	ProxyIntegrationNginxFile   = "nginx_file"
)

// Proxy loaded verdicts for ProxyRouteStatus.ProxyLoaded.
const (
	ProxyLoadedUnknown = "unknown"
	ProxyLoadedYes     = "yes"
	ProxyLoadedNo      = "no"
)

// Audit actions for files written into an external proxy's config directory.
const (
	AuditActionFamilyProxyRoute = "proxy_route"
	AuditActionProxyRouteWrite  = AuditActionFamilyProxyRoute + ".written"
	AuditActionProxyRouteRemove = AuditActionFamilyProxyRoute + ".removed"
)

// ProxyIntegrationSettings is the single proxy_integration_settings row.
type ProxyIntegrationSettings struct {
	Mode            string
	DynamicDir      string
	EntrypointHTTP  string
	EntrypointHTTPS string
	CertResolver    string
	UpstreamHost    string
	// Container, CertDir and Webroot are nginx only: the proxy container to
	// validate and reload, the in-container certificate directory and the
	// optional ACME webroot.
	Container string
	CertDir   string
	Webroot   string
}

// Enabled reports whether managed routes are on.
func (s ProxyIntegrationSettings) Enabled() bool {
	return s.Mode == ProxyIntegrationTraefikFile || s.Mode == ProxyIntegrationNginxFile
}

// ValidProxyIntegrationMode reports whether mode is a known value.
func ValidProxyIntegrationMode(mode string) bool {
	return mode == ProxyIntegrationOff || mode == ProxyIntegrationTraefikFile || mode == ProxyIntegrationNginxFile
}

// ProxyRouteStatus is one domain's managed route: what was written and the
// outcome of the last end to end probe.
type ProxyRouteStatus struct {
	Domain       string
	Target       string
	FileName     string
	Written      bool
	WrittenAt    time.Time
	ProxyLoaded  string
	Reachable    bool
	StatusCode   int
	CertIssuer   string
	CertNotAfter time.Time
	CertTrusted  bool
	LastError    string
	VerifiedAt   time.Time
	// Delivered is true when the last verification proved the proxy hands
	// this domain's requests to the built-in ingress.
	Delivered bool
}

// GetProxyIntegrationSettings returns the seeded singleton row.
func (db *DB) GetProxyIntegrationSettings(ctx context.Context) (ProxyIntegrationSettings, error) {
	var s ProxyIntegrationSettings
	err := db.QueryRowContext(ctx, `
		SELECT mode, dynamic_dir, entrypoint_http, entrypoint_https, cert_resolver, upstream_host, container, cert_dir, webroot
		FROM proxy_integration_settings WHERE id = 1
	`).Scan(&s.Mode, &s.DynamicDir, &s.EntrypointHTTP, &s.EntrypointHTTPS, &s.CertResolver, &s.UpstreamHost, &s.Container, &s.CertDir, &s.Webroot)
	if err != nil {
		return ProxyIntegrationSettings{}, fmt.Errorf("store: get proxy integration settings: %w", err)
	}
	return s, nil
}

// UpdateProxyIntegrationSettings replaces the singleton row in full.
func (db *DB) UpdateProxyIntegrationSettings(ctx context.Context, s ProxyIntegrationSettings) error {
	if !ValidProxyIntegrationMode(s.Mode) {
		return fmt.Errorf("store: proxy integration mode %q is not off, traefik_file or nginx_file", s.Mode)
	}
	_, err := db.ExecContext(ctx, `
		UPDATE proxy_integration_settings
		SET mode = ?, dynamic_dir = ?, entrypoint_http = ?, entrypoint_https = ?, cert_resolver = ?, upstream_host = ?,
			container = ?, cert_dir = ?, webroot = ?
		WHERE id = 1
	`, s.Mode, s.DynamicDir, s.EntrypointHTTP, s.EntrypointHTTPS, s.CertResolver, s.UpstreamHost, s.Container, s.CertDir, s.Webroot)
	if err != nil {
		return fmt.Errorf("store: update proxy integration settings: %w", err)
	}
	return nil
}

func formatProxyTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseProxyTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ListProxyRouteStatus returns every tracked managed route, ordered by domain.
func (db *DB) ListProxyRouteStatus(ctx context.Context) ([]ProxyRouteStatus, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT domain, target, file_name, written, written_at, proxy_loaded, reachable, status_code,
		       cert_issuer, cert_not_after, cert_trusted, last_error, verified_at, delivered
		FROM proxy_route_status ORDER BY domain
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list proxy route status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ProxyRouteStatus
	for rows.Next() {
		var (
			s                               ProxyRouteStatus
			written, reachable, trusted     int
			delivered                       int
			writtenAt, verifiedAt, notAfter string
		)
		if err := rows.Scan(&s.Domain, &s.Target, &s.FileName, &written, &writtenAt, &s.ProxyLoaded, &reachable, &s.StatusCode,
			&s.CertIssuer, &notAfter, &trusted, &s.LastError, &verifiedAt, &delivered); err != nil {
			return nil, fmt.Errorf("store: scan proxy route status: %w", err)
		}
		s.Written, s.Reachable, s.CertTrusted, s.Delivered = written != 0, reachable != 0, trusted != 0, delivered != 0
		s.CertNotAfter = parseProxyTime(notAfter)
		s.WrittenAt, s.VerifiedAt = parseProxyTime(writtenAt), parseProxyTime(verifiedAt)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate proxy route status: %w", err)
	}
	return out, nil
}

// MarkProxyRouteWritten records that domain's file now holds the current
// route. changed clears the last verification so the route is probed again.
func (db *DB) MarkProxyRouteWritten(ctx context.Context, domain, target, fileName string, at time.Time, changed bool) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO proxy_route_status (domain, target, file_name, written, written_at)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(domain) DO UPDATE SET
			target = excluded.target,
			file_name = excluded.file_name,
			written = 1,
			written_at = CASE WHEN ? THEN excluded.written_at WHEN proxy_route_status.written_at = '' THEN excluded.written_at ELSE proxy_route_status.written_at END,
			verified_at = CASE WHEN ? THEN '' ELSE proxy_route_status.verified_at END,
			reachable = CASE WHEN ? THEN 0 ELSE proxy_route_status.reachable END,
			delivered = CASE WHEN ? THEN 0 ELSE proxy_route_status.delivered END,
			proxy_loaded = CASE WHEN ? THEN 'unknown' ELSE proxy_route_status.proxy_loaded END,
			last_error = CASE WHEN ? OR proxy_route_status.written = 0 THEN '' ELSE proxy_route_status.last_error END
	`, domain, target, fileName, formatProxyTime(at), changed, changed, changed, changed, changed, changed)
	if err != nil {
		return fmt.Errorf("store: mark proxy route %s written: %w", domain, err)
	}
	return nil
}

// MarkProxyRouteFailed records that domain's route could not be written.
func (db *DB) MarkProxyRouteFailed(ctx context.Context, domain, target, lastError string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO proxy_route_status (domain, target, written, last_error)
		VALUES (?, ?, 0, ?)
		ON CONFLICT(domain) DO UPDATE SET target = excluded.target, written = 0, delivered = 0, last_error = excluded.last_error
	`, domain, target, lastError)
	if err != nil {
		return fmt.Errorf("store: mark proxy route %s failed: %w", domain, err)
	}
	return nil
}

// SaveProxyRouteVerification stores a probe outcome for an already tracked
// domain. A domain removed meanwhile is left untracked.
func (db *DB) SaveProxyRouteVerification(ctx context.Context, s ProxyRouteStatus) error {
	_, err := db.ExecContext(ctx, `
		UPDATE proxy_route_status SET proxy_loaded = ?, reachable = ?, status_code = ?, cert_issuer = ?,
			cert_not_after = ?, cert_trusted = ?, last_error = ?, verified_at = ?, delivered = ?
		WHERE domain = ?
	`, s.ProxyLoaded, boolInt(s.Reachable), s.StatusCode, s.CertIssuer, formatProxyTime(s.CertNotAfter), boolInt(s.CertTrusted),
		s.LastError, formatProxyTime(s.VerifiedAt), boolInt(s.Delivered), s.Domain)
	if err != nil {
		return fmt.Errorf("store: save proxy route %s verification: %w", s.Domain, err)
	}
	return nil
}

// DeleteProxyRouteStatus forgets domain's managed route.
func (db *DB) DeleteProxyRouteStatus(ctx context.Context, domain string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM proxy_route_status WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("store: delete proxy route %s status: %w", domain, err)
	}
	return nil
}
