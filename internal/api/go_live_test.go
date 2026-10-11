package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

// goLiveFixture is a router with a fake DNS provider managing example.com,
// a known public address and deterministic network probes.
type goLiveFixture struct {
	rt     *Router
	db     *store.DB
	mgr    *fakeDNSManager
	cookie *http.Cookie
}

func newGoLiveFixture(t *testing.T, publicHost string, recs ...libdns.Record) *goLiveFixture {
	t.Helper()
	rt, db := newTestRouter(t)
	mgr := &fakeDNSManager{recs: recs}
	rt.publicHost = publicHost
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.domainAuto.resolveTarget = func(_ context.Context, domain string) (*dnsTarget, error) {
		if !strings.HasSuffix(domain, "example.com") {
			return nil, dnsrecords.ErrZoneNotFound
		}
		return &dnsTarget{Manager: mgr, Provider: "fake", Zone: "example.com."}, nil
	}
	rt.domainAuto.tlsProbe = func(context.Context, string, string) (tlsProbeResult, error) {
		return tlsProbeResult{Issuer: "Let's Encrypt R11", NotAfter: time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), Trusted: true}, nil
	}
	rt.domainAuto.httpProbe = func(context.Context, string, string) (int, error) { return http.StatusOK, nil }
	rt.domainAuto.proxy = func(context.Context, *http.Request) *proxyIntegrationView { return nil }
	rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{publicHost}, nil }
	rt.publicLookup = func(context.Context, string) []resolverResult {
		return []resolverResult{{Name: "1.1.1.1", Addresses: []string{publicHost}}}
	}
	cookie := loginTestSession(t, rt, db)
	return &goLiveFixture{rt: rt, db: db, mgr: mgr, cookie: cookie}
}

func (f *goLiveFixture) setPolicy(t *testing.T, raw string) {
	t.Helper()
	if err := f.db.SetDomainAutomationRaw(context.Background(), raw); err != nil {
		t.Fatalf("set policy: %v", err)
	}
}

func (f *goLiveFixture) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookie, method, target, body))
	return rec
}

func (f *goLiveFixture) patchDomains(t *testing.T, body string) editDomainsResponse {
	t.Helper()
	rec := f.do(t, http.MethodPatch, "/api/v1/apps/web/domains", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d body = %s", rec.Code, rec.Body.String())
	}
	var out editDomainsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func rrOf(name, data string) libdns.Record {
	return libdns.RR{Name: name, Type: "A", Data: data, TTL: time.Minute}
}

func TestAddDomain_AutomaticDNS(t *testing.T) {
	tests := []struct {
		name        string
		publicHost  string
		existing    []libdns.Record
		body        string
		wantDNS     string
		wantRecords int
		wantType    string
	}{
		{"creates an A record", "203.0.113.5", nil, `{"add":["app.example.com"]}`, "created", 1, "A"},
		{"identical record is unchanged", "203.0.113.5", []libdns.Record{rrOf("app", "203.0.113.5")}, `{"add":["app.example.com"]}`, "unchanged", 1, "A"},
		{"foreign record is a conflict and untouched", "203.0.113.5", []libdns.Record{rrOf("app", "198.51.100.9")}, `{"add":["app.example.com"]}`, "conflict", 1, "A"},
		{"replace overwrites the conflict", "203.0.113.5", []libdns.Record{rrOf("app", "198.51.100.9")}, `{"add":["app.example.com"],"replace":true}`, "updated", 1, "A"},
		{"dns off skips", "203.0.113.5", nil, `{"add":["app.example.com"],"dns":"off"}`, "skipped", 0, ""},
		{"preview writes nothing", "203.0.113.5", nil, `{"add":["app.example.com"],"dns":"preview"}`, "preview", 0, ""},
		{"ipv6 only host gets AAAA", "2001:db8::5", nil, `{"add":["app.example.com"]}`, "created", 1, "AAAA"},
		{"unmanaged zone is skipped", "203.0.113.5", nil, `{"add":["app.other.org"]}`, "skipped", 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newGoLiveFixture(t, tc.publicHost, tc.existing...)
			f.setPolicy(t, `{"auto_dns":true}`)
			seedAppWithDomains(t, f.db)

			out := f.patchDomains(t, tc.body)
			if len(out.DNSResults) == 0 || out.DNSResults[0].DNS != tc.wantDNS {
				t.Fatalf("dns results = %+v, want %s", out.DNSResults, tc.wantDNS)
			}
			if len(f.mgr.recs) != tc.wantRecords {
				t.Fatalf("records in zone = %d, want %d: %+v", len(f.mgr.recs), tc.wantRecords, f.mgr.recs)
			}
			if tc.wantType != "" && f.mgr.recs[0].RR().Type != tc.wantType {
				t.Fatalf("record type = %s, want %s", f.mgr.recs[0].RR().Type, tc.wantType)
			}
			if tc.wantDNS == "conflict" && f.mgr.recs[0].RR().Data != "198.51.100.9" {
				t.Fatal("a conflicting foreign record was overwritten without replace")
			}
		})
	}
}

func TestAddDomain_TracksAuditsAndRemovesOnlyOwnRecords(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5", rrOf("www", "198.51.100.7"))
	f.setPolicy(t, `{"auto_dns":true}`)
	seedAppWithDomains(t, f.db)
	ctx := context.Background()

	f.patchDomains(t, `{"add":["app.example.com","www.example.com"]}`)
	rows, err := f.db.ListManagedDNSRecords(ctx, "app.example.com")
	if err != nil || len(rows) != 1 || rows[0].Value != "203.0.113.5" {
		t.Fatalf("managed rows = %+v err = %v", rows, err)
	}
	if foreign, _ := f.db.ListManagedDNSRecords(ctx, "www.example.com"); len(foreign) != 0 {
		t.Fatalf("a record Levelrail did not create must not be tracked: %+v", foreign)
	}
	entries, err := f.db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: store.AuditActionDNSRecordCreated})
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries = %d err = %v, want 1 dns_record.created", len(entries), err)
	}

	out := f.patchDomains(t, `{"remove":["app.example.com","www.example.com"],"remove_dns":true}`)
	deleted := 0
	for _, d := range out.DNSResults {
		if d.DNS == "deleted" {
			deleted++
		}
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want only the one record Levelrail created: %+v", deleted, out.DNSResults)
	}
	if len(f.mgr.recs) != 1 || f.mgr.recs[0].RR().Data != "198.51.100.7" {
		t.Fatalf("the foreign record must survive: %+v", f.mgr.recs)
	}
}

func TestGoLiveStatusAndRun(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	f.setPolicy(t, `{"auto_dns":true}`)
	seedAppWithDomains(t, f.db)

	rec := f.do(t, http.MethodPost, "/api/v1/apps/web/domains/app.example.com/go-live", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d body = %s", rec.Code, rec.Body.String())
	}
	var res goLiveResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.State != goLiveLive {
		t.Fatalf("state = %s steps = %+v", res.State, res.Steps)
	}
	if res.RunID == "" || !res.Undoable {
		t.Fatalf("run must be recorded and undoable: %+v", res)
	}
	var cert goLiveStep
	for _, s := range res.Steps {
		if s.ID == stepCertificate {
			cert = s
		}
	}
	if cert.State != stepDone || cert.Issuer != "Let's Encrypt R11" || cert.NotAfter == nil {
		t.Fatalf("certificate step = %+v", cert)
	}

	rec = f.do(t, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/go-live", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var status goLiveResult
	_ = json.Unmarshal(rec.Body.Bytes(), &status)
	if status.State != goLiveLive || status.Steps[0].ID != stepDNS || status.Steps[0].State != stepDone {
		t.Fatalf("status result = %+v", status)
	}

	undo := f.do(t, http.MethodPost, "/api/v1/domains/automation/runs/"+res.RunID+"/undo", "")
	if undo.Code != http.StatusOK {
		t.Fatalf("undo status = %d body = %s", undo.Code, undo.Body.String())
	}
	if len(f.mgr.recs) != 0 {
		t.Fatalf("undo must remove the created record: %+v", f.mgr.recs)
	}
	if again := f.do(t, http.MethodPost, "/api/v1/domains/automation/runs/"+res.RunID+"/undo", ""); again.Code != http.StatusConflict {
		t.Fatalf("second undo status = %d, want 409", again.Code)
	}
}

func TestGoLiveStepStates(t *testing.T) {
	tests := []struct {
		name      string
		lookup    []string
		public    []string
		tls       tlsProbeResult
		tlsErr    error
		wantState string
		wantProp  string
		wantCert  string
	}{
		{"fully live", []string{"203.0.113.5"}, []string{"203.0.113.5"}, tlsProbeResult{Issuer: "R11", Trusted: true, NotAfter: time.Now().Add(time.Hour)}, nil, goLiveLive, stepDone, stepDone},
		{"propagating", nil, []string{"203.0.113.5"}, tlsProbeResult{}, errNoCert, goLivePending, stepPending, stepPending},
		{"internal certificate", []string{"203.0.113.5"}, nil, tlsProbeResult{Issuer: "Caddy Local Authority"}, nil, goLivePending, stepDone, stepPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newGoLiveFixture(t, "203.0.113.5")
			f.setPolicy(t, `{"auto_dns":false}`)
			seedAppWithDomains(t, f.db, "app.example.com")
			f.rt.lookupHost = func(context.Context, string) ([]string, error) {
				if tc.lookup == nil {
					return nil, errNoCert
				}
				return tc.lookup, nil
			}
			f.rt.publicLookup = func(context.Context, string) []resolverResult {
				return []resolverResult{{Name: "1.1.1.1", Addresses: tc.public}}
			}
			f.rt.domainAuto.tlsProbe = func(context.Context, string, string) (tlsProbeResult, error) { return tc.tls, tc.tlsErr }

			rec := f.do(t, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/go-live", "")
			var res goLiveResult
			_ = json.Unmarshal(rec.Body.Bytes(), &res)
			got := map[string]string{}
			for _, s := range res.Steps {
				got[s.ID] = s.State
			}
			if res.State != tc.wantState || got[stepPropagation] != tc.wantProp || got[stepCertificate] != tc.wantCert {
				t.Fatalf("state=%s steps=%v, want %s prop=%s cert=%s", res.State, got, tc.wantState, tc.wantProp, tc.wantCert)
			}
		})
	}
}

var errNoCert = &net404{}

type net404 struct{}

func (*net404) Error() string { return "no answer" }

func TestGoLivePlanIsADryRun(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	f.setPolicy(t, `{"auto_dns":true,"www_policy":"redirect_to_apex"}`)
	seedAppWithDomains(t, f.db)

	rec := f.do(t, http.MethodPost, "/api/v1/apps/web/domains/go-live/plan", `{"domain":"example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var out goLivePlanResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Plans) != 1 || out.Plans[0].State != goLivePlanned {
		t.Fatalf("plan = %+v", out)
	}
	ids := strings.Join(stepIDs(out.Plans[0].Steps), ",")
	if !strings.Contains(ids, stepDNS) || !strings.Contains(ids, stepCounterpart) || !strings.Contains(ids, stepRedirect) {
		t.Fatalf("steps = %s", ids)
	}
	if len(f.mgr.recs) != 0 {
		t.Fatalf("a plan must not write records: %+v", f.mgr.recs)
	}
}

func TestGoLiveWWWCounterpartAndRedirect(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	f.setPolicy(t, `{"auto_dns":true,"www_policy":"redirect_to_apex"}`)
	seedAppWithDomains(t, f.db)

	out := f.patchDomains(t, `{"add":["example.com"]}`)
	if len(out.GoLive) != 1 {
		t.Fatalf("go_live = %+v", out.GoLive)
	}
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.Domains) != 2 {
		t.Fatalf("domains = %v, want apex and www", svc.Domains)
	}
	row, found, err := f.db.GetDomainRedirect(context.Background(), "www.example.com")
	if err != nil || !found || row.TargetURL != "https://example.com" {
		t.Fatalf("redirect = %+v found=%v err=%v", row, found, err)
	}
	if len(f.mgr.recs) != 2 {
		t.Fatalf("records = %+v, want apex and www", f.mgr.recs)
	}
}

func TestCounterpartHost(t *testing.T) {
	tests := []struct {
		domain, rel string
		known       bool
		want        string
		isWWW, ok   bool
	}{
		{"example.com", "@", true, "www.example.com", false, true},
		{"www.example.com", "www", true, "example.com", true, true},
		{"app.example.com", "app", true, "", false, false},
		{"example.com", "", false, "www.example.com", false, true},
		{"app.example.com", "", false, "", false, false},
	}
	for _, tc := range tests {
		got, isWWW, ok := counterpartHost(tc.domain, tc.rel, tc.known)
		if got != tc.want || isWWW != tc.isWWW || ok != tc.ok {
			t.Errorf("counterpartHost(%q) = %q %v %v", tc.domain, got, isWWW, ok)
		}
	}
}

func TestAppsBaseDomain(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	f.setPolicy(t, `{"auto_dns":true}`)

	bad := f.do(t, http.MethodPut, "/api/v1/settings/ingress", `{"apps_base_domain":"apps.unmanaged.org"}`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unmanaged zone status = %d body = %s", bad.Code, bad.Body.String())
	}
	invalid := f.do(t, http.MethodPut, "/api/v1/settings/ingress", `{"apps_base_domain":"not a host"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid hostname status = %d", invalid.Code)
	}
	ok := f.do(t, http.MethodPut, "/api/v1/settings/ingress", `{"apps_base_domain":"Apps.Example.com"}`)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"apps_base_domain":"apps.example.com"`) {
		t.Fatalf("set status = %d body = %s", ok.Code, ok.Body.String())
	}

	create := f.do(t, http.MethodPost, "/api/v1/apps", `{"name":"shop","image":"levelrail/shop:1","port":8080}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}
	var app appResource
	_ = json.Unmarshal(create.Body.Bytes(), &app)
	if app.AutoDomain != "shop.apps.example.com" || len(app.Domains) != 1 || app.DNS == nil || app.DNS.DNS != "created" {
		t.Fatalf("created app = %+v dns = %+v", app, app.DNS)
	}
	if len(f.mgr.recs) != 1 || f.mgr.recs[0].RR().Name != "shop.apps" {
		t.Fatalf("records = %+v", f.mgr.recs)
	}

	explicit := f.do(t, http.MethodPost, "/api/v1/apps", `{"name":"blog","image":"levelrail/blog:1","port":8080,"domains":["blog.example.com"]}`)
	var blog appResource
	_ = json.Unmarshal(explicit.Body.Bytes(), &blog)
	if blog.AutoDomain != "" {
		t.Fatalf("an app with its own domain must not get the base domain: %+v", blog)
	}
}

func TestBackfillBaseDomain(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	f.setPolicy(t, `{"auto_dns":true}`)
	if err := f.db.SaveDesiredService(context.Background(), store.DesiredService{Name: "old", Image: "levelrail/old:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if rec := f.do(t, http.MethodPut, "/api/v1/settings/ingress", `{"apps_base_domain":"apps.example.com"}`); rec.Code != http.StatusOK {
		t.Fatalf("set base: %d %s", rec.Code, rec.Body.String())
	}

	dry := f.do(t, http.MethodPost, "/api/v1/settings/ingress/apps-base-domain/backfill", `{}`)
	var out baseDomainBackfillResponse
	_ = json.Unmarshal(dry.Body.Bytes(), &out)
	if !out.DryRun || len(out.Items) != 1 || out.Items[0].Status != "planned" || len(f.mgr.recs) != 0 {
		t.Fatalf("dry run = %+v records=%v", out, f.mgr.recs)
	}
	applied := f.do(t, http.MethodPost, "/api/v1/settings/ingress/apps-base-domain/backfill", `{"confirm":true}`)
	_ = json.Unmarshal(applied.Body.Bytes(), &out)
	if out.DryRun || out.Items[0].Status != "attached" || len(f.mgr.recs) != 1 {
		t.Fatalf("applied = %+v records=%v", out, f.mgr.recs)
	}
}

func TestDomainAutomationSettings(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	rec := f.do(t, http.MethodGet, "/api/v1/settings/domain-automation", "")
	var res domainAutomationResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if !res.ForceHTTPS || !res.VerifyAfter || res.WWWPolicy != wwwPolicyOff || res.AutoDNS {
		t.Fatalf("defaults = %+v", res)
	}
	if bad := f.do(t, http.MethodPut, "/api/v1/settings/domain-automation", `{"www_policy":"sideways"}`); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid www_policy status = %d", bad.Code)
	}
	if ok := f.do(t, http.MethodPut, "/api/v1/settings/domain-automation", `{"www_policy":"redirect_to_www","auto_dns":true}`); ok.Code != http.StatusOK {
		t.Fatalf("put status = %d", ok.Code)
	}
	_ = json.Unmarshal(f.do(t, http.MethodGet, "/api/v1/settings/domain-automation", "").Body.Bytes(), &res)
	if res.WWWPolicy != wwwPolicyRedirectToWWW || !res.AutoDNS {
		t.Fatalf("after put = %+v", res)
	}
}

func TestValidateHostname(t *testing.T) {
	tests := map[string]bool{
		"apps.example.com": true, "a-b.example.co.uk": true,
		"example": false, "*.example.com": false, "Apps.example.com": false, "-a.example.com": false, "": false, "a..com": false,
	}
	for in, ok := range tests {
		if err := validateHostname(in); (err == nil) != ok {
			t.Errorf("validateHostname(%q) err = %v, want ok=%v", in, err, ok)
		}
	}
}
