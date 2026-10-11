package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

type tlsOwnerFixture struct {
	rt        *Router
	cookie    *http.Cookie
	notAfter  time.Time
	proxyHost string
}

// newTLSOwnerFixture seeds a domain with a stale ACME failure and a Levelrail
// certificate close to expiry, the state a server is left in after a proxy
// takes over TLS.
func newTLSOwnerFixture(t *testing.T, upstream, routeTrusted bool) tlsOwnerFixture {
	t.Helper()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const host = "app.example.com"
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080, Domains: []string{host}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seedCert(t, db, host, now.Add(-80*24*time.Hour), now.Add(3*24*time.Hour))

	settings, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ACMEEnabled, settings.TLSTerminatedUpstream = true, upstream
	if err := db.UpdateIngressSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	failures := ingress.DefaultACMEFailures()
	failures.Record(ingress.ACMEFailure{Identifier: host, Error: "tls: unrecognized name (ca=acme-staging)", At: now.Add(-time.Hour)})
	t.Cleanup(failures.ClearAll)

	notAfter := now.Add(70 * 24 * time.Hour).UTC().Truncate(time.Second)
	rt.proxyIntegration = &proxyIntegrationDeps{store: fakeProxyRouteStatuses{rows: []store.ProxyRouteStatus{{
		Domain: host, Written: true, Reachable: true, CertTrusted: routeTrusted, CertIssuer: "R11", CertNotAfter: notAfter,
	}}}}
	return tlsOwnerFixture{rt: rt, cookie: loginTestSession(t, rt, db), notAfter: notAfter, proxyHost: host}
}

func TestProxyManagedDomainReportsOneCoherentState(t *testing.T) {
	f := newTLSOwnerFixture(t, true, true)

	var certs []certificateStatus
	if code := trafficCall(t, f.rt, f.cookie, http.MethodGet, "/api/v1/certificates", &certs); code != http.StatusOK {
		t.Fatalf("certificates status = %d", code)
	}
	if len(certs) != 1 {
		t.Fatalf("certificates = %+v, want only the proxy-served one", certs)
	}
	c := certs[0]
	if c.Source != certSourceProxy || c.TLSOwner != "proxy" || c.Issuer != "R11" || !c.NotAfter.Equal(f.notAfter) ||
		c.Status != "healthy" || c.Renewal != "ok" || c.ACMEFailure != nil {
		t.Errorf("certificate = %+v", c)
	}

	var domains []domainResource
	if code := trafficCall(t, f.rt, f.cookie, http.MethodGet, "/api/v1/domains", &domains); code != http.StatusOK {
		t.Fatalf("domains status = %d", code)
	}
	var row domainResource
	for _, d := range domains {
		if d.Domain == f.proxyHost {
			row = d
		}
	}
	if row.TLSOwner != "proxy" || row.ACMEFailure != nil || row.ProxyCertificate == nil ||
		row.ProxyCertificate.Issuer != "R11" || !row.ProxyCertificate.NotAfter.Equal(f.notAfter) || row.ProxyCertificate.Status != "healthy" {
		t.Errorf("domain row = %+v", row)
	}

	var sum trafficSummaryResource
	if code := trafficCall(t, f.rt, f.cookie, http.MethodGet, "/api/v1/traffic/summary", &sum); code != http.StatusOK {
		t.Fatalf("summary status = %d", code)
	}
	if sum.Certificates.Expiring != 0 || sum.Certificates.RenewalFailing != 0 || sum.Domains.NeedsAttention != 0 {
		t.Errorf("summary = %+v, want no expiring, failing or attention", sum)
	}
}

func TestLevelrailOwnedDomainKeepsItsACMEState(t *testing.T) {
	f := newTLSOwnerFixture(t, false, false)

	var certs []certificateStatus
	if code := trafficCall(t, f.rt, f.cookie, http.MethodGet, "/api/v1/certificates", &certs); code != http.StatusOK {
		t.Fatalf("certificates status = %d", code)
	}
	if len(certs) != 1 || certs[0].TLSOwner != "acme" || certs[0].ACMEFailure == nil || certs[0].Status != "expiring_soon" {
		t.Errorf("certificates = %+v", certs)
	}
}

func TestProxyRouteWithoutTrustedCertDoesNotOwnTLS(t *testing.T) {
	f := newTLSOwnerFixture(t, false, false)
	snap := f.rt.tlsSnapshot(context.Background())
	if snap.ProxyManaged(f.proxyHost) {
		t.Fatal("an untrusted route must not take TLS ownership")
	}
}

func TestSecurityPostureCountsProxyServedCertificate(t *testing.T) {
	f := newTLSOwnerFixture(t, true, true)
	in := postureInput{}
	f.rt.collectPostureTLS(context.Background(), &in, time.Now())
	if !in.RealCert || len(in.InsecureDomains) != 0 {
		t.Errorf("posture = RealCert %v insecure %v", in.RealCert, in.InsecureDomains)
	}
}
