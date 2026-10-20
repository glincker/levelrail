package api

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/tlsowner"
)

// certSourceProxy marks a certificate row that is the one a fronting proxy
// serves, not one stored by this control plane.
const certSourceProxy = "proxy"

// servedCertificateResource is the certificate a managing proxy serves.
type servedCertificateResource struct {
	Issuer   string    `json:"issuer,omitempty"`
	NotAfter time.Time `json:"not_after"`
	Status   string    `json:"status"`
}

// tlsSnapshot loads the effective TLS ownership state for this request.
func (rt *Router) tlsSnapshot(ctx context.Context) tlsowner.Snapshot {
	var settings tlsowner.SettingsReader
	if rt.ingressSettings != nil {
		settings = rt.ingressSettings
	}
	var routes tlsowner.RouteReader
	if rt.proxyIntegration != nil && rt.proxyIntegration.store != nil {
		routes = rt.proxyIntegration.store
	}
	var custom map[string]bool
	if rt.domainTLSCert != nil {
		if c, err := rt.customTLSCertDomains(ctx); err == nil {
			custom = c
		}
	}
	return tlsowner.Load(ctx, settings, routes, custom)
}

func (rt *Router) certWarningWindow() time.Duration {
	if rt.certExpiryWarningWindow > 0 {
		return rt.certExpiryWarningWindow
	}
	return alerting.DefaultCertExpiryWarningWindow
}

// proxyCertificateFor returns the served certificate for a proxy-owned domain.
func (rt *Router) proxyCertificateFor(snap tlsowner.Snapshot, domain string, now time.Time) *servedCertificateResource {
	if !snap.ProxyManaged(domain) {
		return nil
	}
	pc, ok := snap.ProxyCert(domain)
	if !ok {
		return nil
	}
	return &servedCertificateResource{
		Issuer:   pc.Issuer,
		NotAfter: pc.NotAfter,
		Status:   alerting.CertExpiryStatus(pc.NotAfter, now, rt.certWarningWindow()),
	}
}

// acmeFailureIn is the recorded CA failure for domain, hidden once a proxy
// owns TLS: that attempt is stale and no longer what visitors get.
func acmeFailureIn(snap tlsowner.Snapshot, domain string) *acmeFailureResource {
	if snap.Suppressed(domain) {
		return nil
	}
	return acmeFailureFor(domain)
}

// annotateDomainTLS sets each row's TLS owner and proxy-served certificate
// and drops the stale ACME failure of a proxy-managed host.
func (rt *Router) annotateDomainTLS(ctx context.Context, rows []domainResource) {
	snap := rt.tlsSnapshot(ctx)
	now := time.Now()
	for i := range rows {
		d := rows[i].Domain
		rows[i].TLSOwner = string(snap.Owner(d))
		rows[i].ProxyCertificate = rt.proxyCertificateFor(snap, d, now)
		if snap.Suppressed(d) {
			rows[i].ACMEFailure = nil
		}
	}
}

// proxyCertificateRows turns the certificates proxies serve into rows for
// GET /api/v1/certificates, replacing the stored ones for those hosts.
func (rt *Router) proxyCertificateRows(snap tlsowner.Snapshot, owners map[string][]string, canSee func(string) bool, now time.Time) []certificateStatus {
	hosts := snap.ProxyHosts()
	sort.Strings(hosts)
	var out []certificateStatus
	for _, host := range hosts {
		pc, ok := snap.ProxyCert(host)
		if !ok || !certVisible(host, nil, owners, canSee) {
			continue
		}
		out = append(out, certificateStatus{
			Domain:   host,
			Issuer:   pc.Issuer,
			NotAfter: pc.NotAfter,
			Status:   alerting.CertExpiryStatus(pc.NotAfter, now, rt.certWarningWindow()),
			Renewal:  alerting.CertRenewalOK,
			Apps:     owners[strings.ToLower(host)],
			Source:   certSourceProxy,
			TLSOwner: string(tlsowner.Proxy),
		})
	}
	return out
}
