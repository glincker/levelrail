package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComputeReachability(t *testing.T) {
	base := reachabilityInput{Domain: "shop.example.com", AppRunning: true, Routed: true, CheckStatus: domainCheckStatusConnected}
	cases := []struct {
		name     string
		mut      func(*reachabilityInput)
		url, rsn string
	}{
		{"live", func(*reachabilityInput) {}, "https://shop.example.com", ""},
		{"unknown dns still links", func(i *reachabilityInput) { i.CheckStatus = "" }, "https://shop.example.com", ""},
		{"propagating links", func(i *reachabilityInput) { i.CheckStatus = domainCheckStatusPropagating }, "https://shop.example.com", ""},
		{"public port", func(i *reachabilityInput) { i.LinkPort = 8443 }, "https://shop.example.com:8443", ""},
		{"port 443 omitted", func(i *reachabilityInput) { i.LinkPort = 443 }, "https://shop.example.com", ""},
		{"stopped", func(i *reachabilityInput) { i.AppRunning = false }, "", reachableReasonStopped},
		{"not resolving", func(i *reachabilityInput) { i.CheckStatus = domainCheckStatusNotResolving }, "", reachableReasonNotResolving},
		{"resolves elsewhere", func(i *reachabilityInput) { i.CheckStatus = domainCheckStatusResolvesElsewhere }, "", reachableReasonNotResolving},
		{"elsewhere behind upstream proxy", func(i *reachabilityInput) {
			i.CheckStatus, i.Upstream = domainCheckStatusResolvesElsewhere, true
		}, "https://shop.example.com", ""},
		{"not routed", func(i *reachabilityInput) { i.Routed = false }, "", reachableReasonNotRouted},
		{"held back wins", func(i *reachabilityInput) { i.HeldBack, i.AppRunning = true, false }, "", reachableReasonHeldBack},
		{"stopped before dns", func(i *reachabilityInput) {
			i.AppRunning, i.CheckStatus = false, domainCheckStatusNotResolving
		}, "", reachableReasonStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mut(&in)
			u, r := computeReachability(in)
			if u != tc.url || r != tc.rsn {
				t.Errorf("got (%q, %q), want (%q, %q)", u, r, tc.url, tc.rsn)
			}
		})
	}
}

func TestPublicLinkPort(t *testing.T) {
	cases := []struct {
		name     string
		listen   int
		settings store.IngressSettings
		port     int
		routed   bool
	}{
		{"default 443", 0, store.IngressSettings{}, 0, true},
		{"listen 443", 443, store.IngressSettings{}, 0, true},
		{"listen 8443 no public port", 8443, store.IngressSettings{}, 0, false},
		{"listen 8443 public 443 upstream", 8443, store.IngressSettings{TLSTerminatedUpstream: true}, 443, true},
		{"explicit public port", 8443, store.IngressSettings{PublicHTTPSPort: 9443}, 9443, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{doctorHTTPSPort: tc.listen}
			p, ok := rt.publicLinkPort(tc.settings)
			if p != tc.port || ok != tc.routed {
				t.Errorf("got (%d, %v), want (%d, %v)", p, ok, tc.port, tc.routed)
			}
		})
	}
}

func TestClassifyDomain(t *testing.T) {
	expired := &certSignal{Status: certStatusExpired}
	stalled := &certSignal{Status: certStatusExpiringSoon, Renewal: alerting.CertRenewalStalled}
	healthy := &certSignal{Status: "healthy", Renewal: alerting.CertRenewalOK}
	cases := []struct {
		name string
		in   domainSignals
		want string
	}{
		{"unknown", domainSignals{}, trafficClassUnknown},
		{"live", domainSignals{CheckStatus: domainCheckStatusConnected, Cert: healthy}, trafficClassLive},
		{"paused beats everything", domainSignals{Maintenance: true, Cert: expired}, trafficClassPaused},
		{"expired cert", domainSignals{CheckStatus: domainCheckStatusConnected, Cert: expired}, trafficClassNeedsAttention},
		{"stalled renewal", domainSignals{CheckStatus: domainCheckStatusConnected, Cert: stalled}, trafficClassNeedsAttention},
		{"acme failure", domainSignals{CheckStatus: domainCheckStatusConnected, ACMEAction: acmeActionOpenPort80}, trafficClassNeedsAttention},
		{"rate limited is not attention", domainSignals{CheckStatus: domainCheckStatusConnected, ACMEAction: acmeActionWaitRetry}, trafficClassLive},
		{"resolves elsewhere", domainSignals{CheckStatus: domainCheckStatusResolvesElsewhere}, trafficClassNeedsAttention},
		{"elsewhere behind proxy", domainSignals{CheckStatus: domainCheckStatusResolvesElsewhere, Upstream: true}, trafficClassLive},
		{"unconfigured", domainSignals{CheckStatus: domainCheckStatusUnconfigured}, trafficClassNotSetUp},
		{"not resolving", domainSignals{CheckStatus: domainCheckStatusNotResolving}, trafficClassWaiting},
		{"propagating", domainSignals{CheckStatus: domainCheckStatusPropagating}, trafficClassPropagating},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDomain(tc.in); got != tc.want {
				t.Errorf("classifyDomain = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSummarizeCountsVisibleDomains(t *testing.T) {
	snap := &trafficSnapshot{at: time.Now(), windowDays: 30, domains: []trafficDomainState{
		{Domain: "a", App: "web", Class: trafficClassLive, CertExpiring: true},
		{Domain: "b", App: "web", Class: trafficClassWaiting},
		{Domain: "c", App: "web", Class: trafficClassNeedsAttention, CertExpired: true, CertFailing: true},
		{Domain: "d", App: "web", Class: trafficClassPropagating},
		{Domain: "e", App: "secret", Class: trafficClassNeedsAttention},
		{Domain: "f", App: "web", Class: trafficClassUnknown},
	}}
	got := summarize(snap, func(app string) bool { return app == "web" })
	want := trafficDomainCounts{Total: 5, Live: 1, Waiting: 1, Propagating: 1, NeedsAttention: 1, Unknown: 1}
	if got.Domains != want {
		t.Errorf("domains = %+v, want %+v", got.Domains, want)
	}
	if got.Attention != 1 || got.DomainsNotResolving != 1 {
		t.Errorf("attention = %d, not resolving = %d, want 1 and 1", got.Attention, got.DomainsNotResolving)
	}
	if got.Certificates != (trafficCertCounts{WindowDays: 30, Expiring: 1, Expired: 1, RenewalFailing: 1}) {
		t.Errorf("certificates = %+v", got.Certificates)
	}
	if got.ZonesNotDelegated != nil || len(got.Unavailable) != 2 {
		t.Errorf("zones/routes should be unavailable, got %v", got.Unavailable)
	}
}

func seedRunningApp(t *testing.T, db *store.DB, domains ...string) {
	const name = "web"
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "levelrail/" + name + ":1", Port: 3000, Domains: domains}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpsertConditions(ctx, applicationControllerName(name), []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"}}); err != nil {
		t.Fatalf("seed conditions: %v", err)
	}
}

func trafficCall(t *testing.T, rt *Router, cookie *http.Cookie, method, target string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, ""))
	if rec.Code == http.StatusOK && out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v (%s)", target, err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestTrafficSummaryEndpoint(t *testing.T) {
	t.Setenv(envTrafficSummaryCacheTTL, "1h")
	rt, db := newTestRouter(t)
	seedRunningApp(t, db, "live.example.com", "wait.example.com", "prop.example.com", "new.example.com")
	rt.traffic.recordCheck("live.example.com", domainCheckStatusConnected)
	rt.traffic.recordCheck("wait.example.com", domainCheckStatusNotResolving)
	rt.traffic.recordCheck("prop.example.com", domainCheckStatusPropagating)
	cookie := loginTestSession(t, rt, db)

	var got trafficSummaryResource
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/traffic/summary", &got); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	want := trafficDomainCounts{Total: 4, Live: 1, Waiting: 1, Propagating: 1, Unknown: 1}
	if got.Domains != want {
		t.Errorf("domains = %+v, want %+v", got.Domains, want)
	}
	if got.Certificates.WindowDays != defaultTrafficCertDays {
		t.Errorf("window_days = %d", got.Certificates.WindowDays)
	}

	rt.traffic.recordCheck("new.example.com", domainCheckStatusConnected)
	var cached trafficSummaryResource
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/traffic/summary", &cached)
	if cached.Domains.Unknown != 1 {
		t.Errorf("summary was rebuilt inside the cache TTL: %+v", cached.Domains)
	}
}

func TestListDomainsReachableURL(t *testing.T) {
	rt, db := newTestRouter(t)
	seedRunningApp(t, db, "up.example.com", "dark.example.com")
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "idle", Image: "x:1", Port: 80, Domains: []string{"idle.example.com"}}); err != nil {
		t.Fatal(err)
	}
	rt.traffic.recordCheck("dark.example.com", domainCheckStatusNotResolving)
	cookie := loginTestSession(t, rt, db)

	var rows []domainResource
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains", &rows); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	byDomain := map[string]domainResource{}
	for _, r := range rows {
		byDomain[r.Domain] = r
	}
	if r := byDomain["up.example.com"]; r.ReachableURL != "https://up.example.com" || r.ReachableReason != "" {
		t.Errorf("up = %+v", r)
	}
	if r := byDomain["dark.example.com"]; r.ReachableURL != "" || r.ReachableReason != reachableReasonNotResolving {
		t.Errorf("dark = %+v", r)
	}
	if r := byDomain["idle.example.com"]; r.ReachableReason != reachableReasonStopped {
		t.Errorf("idle = %+v", r)
	}
}

func TestDomainDoctorEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.publicHost = "203.0.113.7"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	seedRunningApp(t, db, "far.example.com")
	tlsCalled := false
	rt.traffic.doctorProbes = &domaindoctor.Probes{
		DNS:  doctorFakeDNS{ips: []string{"198.51.100.4"}},
		TLS:  doctorFakeTLS{called: &tlsCalled},
		HTTP: doctorFakeTLS{called: &tlsCalled},
	}
	cookie := loginTestSession(t, rt, db)

	var rep domaindoctor.Report
	if code := trafficCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/domains/far.example.com/doctor", &rep); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if rep.Probed || tlsCalled {
		t.Error("doctor connected to a domain that does not resolve to this server")
	}
	if rep.Status != domaindoctor.StatusProblems || rep.ProbeNote == "" {
		t.Errorf("status = %q note = %q", rep.Status, rep.ProbeNote)
	}
	if code := trafficCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/domains/other.example.com/doctor", nil); code != http.StatusNotFound {
		t.Errorf("foreign domain status = %d, want 404", code)
	}
}

type doctorFakeDNS struct{ ips []string }

func (f doctorFakeDNS) Answers(context.Context, string) []domaindoctor.ResolverAnswer {
	return []domaindoctor.ResolverAnswer{{Resolver: "1.1.1.1", IPv4: f.ips}}
}
func (doctorFakeDNS) CNAME(context.Context, string) (string, error) { return "", nil }
func (doctorFakeDNS) CAA(context.Context, string) ([]domaindoctor.CAARecord, error) {
	return nil, nil
}

type doctorFakeTLS struct{ called *bool }

func (f doctorFakeTLS) Handshake(context.Context, string, string) (domaindoctor.PeerCert, error) {
	*f.called = true
	return domaindoctor.PeerCert{}, nil
}

func (f doctorFakeTLS) Get(context.Context, string, string, string, string) (domaindoctor.HTTPResult, error) {
	*f.called = true
	return domaindoctor.HTTPResult{}, nil
}

func TestAuditResourcePatterns(t *testing.T) {
	cases := []struct {
		in      string
		n       int
		wantErr bool
	}{
		{"domain:shop.example.com", 3, false},
		{"zone:example.com", 2, false},
		{"app:web", 2, false},
		{"domain:", 0, true},
		{"node:x", 0, true},
		{"app:../x/y", 0, true},
		{"nocolon", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := auditResourcePatterns(tc.in)
			if (err != nil) != tc.wantErr || len(got) != tc.n {
				t.Errorf("auditResourcePatterns(%q) = %v, %v", tc.in, got, err)
			}
		})
	}
}

func seedAudit(t *testing.T, db *store.DB, id, at, method, path, ability string, status int) {
	t.Helper()
	if err := db.SaveAuditEntry(context.Background(), store.AuditEntry{ID: id, ActorType: "session", ActorName: "admin@example.com",
		Ability: ability, Method: method, Path: path, StatusCode: status, CreatedAt: at, ClientKind: "dashboard"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogResourceFilterAndDomainActivity(t *testing.T) {
	rt, db := newTestRouter(t)
	seedRunningApp(t, db, "shop.example.com")
	cookie := loginTestSession(t, rt, db)
	seedAudit(t, db, "e1", "2026-10-01T10:00:00.000000000Z", http.MethodPut, "/api/v1/apps/web/domains/shop.example.com/redirect", "deploy", 200)
	seedAudit(t, db, "e2", "2026-10-02T10:00:00.000000000Z", "EVENT", "/api/v1/certificates/shop.example.com", "cert.issued", 200)
	seedAudit(t, db, "e3", "2026-10-03T10:00:00.000000000Z", http.MethodPut, "/api/v1/apps/web/domains/other.example.com/redirect", "deploy", 200)
	seedAudit(t, db, "e4", "2026-10-04T10:00:00.000000000Z", http.MethodDelete, "/api/v1/apps/web/domains/shop.example.com/waf", "deploy", 500)

	var rows []auditLogEntryResource
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/audit-log?resource=domain:shop.example.com", &rows); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(rows) != 3 {
		t.Errorf("audit rows = %d, want 3", len(rows))
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/audit-log?resource=bogus", nil); code != http.StatusBadRequest {
		t.Errorf("bad resource status = %d, want 400", code)
	}

	var page activityPage
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.com/activity?limit=2", &page); code != http.StatusOK {
		t.Fatalf("activity status = %d", code)
	}
	if len(page.Events) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	if e := page.Events[0]; e.Title != "WAF removed" || !e.Failed || e.Kind != activityKindSettings {
		t.Errorf("first event = %+v", e)
	}
	if e := page.Events[1]; e.Title != "Certificate issued" || e.Kind != activityKindCertificate {
		t.Errorf("second event = %+v", e)
	}
	var next activityPage
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.com/activity?limit=2&before="+page.NextCursor, &next)
	if len(next.Events) != 1 || next.Events[0].Title != "Redirect updated" || next.NextCursor != "" {
		t.Errorf("next page = %+v", next)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/nobody.example.com/activity", nil); code != http.StatusOK {
		t.Errorf("root caller should read an unowned domain's history, got %d", code)
	}
}
