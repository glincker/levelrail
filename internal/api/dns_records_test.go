package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

// fakeDNSManager is an in-memory dnsrecords.Manager for this file's own
// tests, so none of them call Cloudflare or Route53's real APIs: the
// same "seam, not an interface" tradeoff lookupHostFunc already accepts
// for a real DNS query (domain_check_test.go).
type fakeDNSManager struct {
	recs []libdns.Record
}

func (m *fakeDNSManager) GetRecords(_ context.Context, _ string) ([]libdns.Record, error) {
	return append([]libdns.Record{}, m.recs...), nil
}

func (m *fakeDNSManager) AppendRecords(_ context.Context, _ string, recs []libdns.Record) ([]libdns.Record, error) {
	m.recs = append(m.recs, recs...)
	return recs, nil
}

func (m *fakeDNSManager) SetRecords(_ context.Context, _ string, recs []libdns.Record) ([]libdns.Record, error) {
	m.recs = recs
	return recs, nil
}

func (m *fakeDNSManager) DeleteRecords(_ context.Context, _ string, recs []libdns.Record) ([]libdns.Record, error) {
	var kept []libdns.Record
	for _, existing := range m.recs {
		remove := false
		for _, target := range recs {
			if existing.RR() == target.RR() {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, existing)
		}
	}
	m.recs = kept
	return recs, nil
}

// newTestRouterWithDNSRecordManager wires a fake, in-memory DNS record
// manager and a deterministic status function, overriding the two
// unexported seam fields directly, the same pattern
// newTestRouterWithLookupHost establishes in domain_check_test.go.
func newTestRouterWithDNSRecordManager(t *testing.T, mgr dnsrecords.Manager) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.dnsRecordManager = func(_ context.Context, domain string) (dnsrecords.Manager, string, string, error) {
		return mgr, "fake", dnsZoneApexGuess(domain), nil
	}
	rt.dnsRecordStatus = func(_ context.Context, _ string, record dnsRecordResource) string {
		if record.Value == "resolved-value" {
			return dnsRecordStatusResolved
		}
		return dnsRecordStatusPending
	}
	return rt, db
}

func TestHandleListDNSRecords_NoProviderConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/dns-records", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleListDNSRecords_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouterWithDNSRecordManager(t, &fakeDNSManager{})
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/dns-records", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListDNSRecords_FiltersNSAndSOAAndReportsStatus(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "@", Type: "NS", Data: "ns1.example.com.", TTL: 3600 * time.Second},
		libdns.RR{Name: "@", Type: "SOA", Data: "ns1.example.com. admin.example.com. 1 7200 900 1209600 86400", TTL: 3600 * time.Second},
		libdns.RR{Name: "app", Type: "A", Data: "resolved-value", TTL: 300 * time.Second},
		libdns.RR{Name: "www", Type: "A", Data: "not-live-yet", TTL: 300 * time.Second},
	}}
	rt, db := newTestRouterWithDNSRecordManager(t, mgr)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/dns-records", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got dnsRecordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2 (NS/SOA filtered out); got %+v", len(got.Records), got.Records)
	}
	if got.Zone != "example.com." {
		t.Errorf("Zone = %q, want %q", got.Zone, "example.com.")
	}
	byName := map[string]dnsRecordResource{}
	for _, r := range got.Records {
		byName[r.Name] = r
	}
	if byName["app"].Status != dnsRecordStatusResolved {
		t.Errorf("app record status = %q, want %q", byName["app"].Status, dnsRecordStatusResolved)
	}
	if byName["www"].Status != dnsRecordStatusPending {
		t.Errorf("www record status = %q, want %q", byName["www"].Status, dnsRecordStatusPending)
	}
}

func TestHandleCreateDNSRecord_Success(t *testing.T) {
	mgr := &fakeDNSManager{}
	rt, db := newTestRouterWithDNSRecordManager(t, mgr)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"www","type":"a","value":"203.0.113.5","ttl_seconds":600}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/domains/app.example.com/dns-records", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(mgr.recs) != 1 {
		t.Fatalf("len(mgr.recs) = %d, want 1", len(mgr.recs))
	}
	rr := mgr.recs[0].RR()
	if rr.Name != "www" || rr.Type != "A" || rr.Data != "203.0.113.5" {
		t.Errorf("appended record = %+v, want name=www type=A data=203.0.113.5", rr)
	}
}

func TestHandleCreateDNSRecord_RejectsUnknownType(t *testing.T) {
	rt, db := newTestRouterWithDNSRecordManager(t, &fakeDNSManager{})
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"www","type":"PTR","value":"203.0.113.5"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/domains/app.example.com/dns-records", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateDNSRecord_ReplacesExactMatch(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "www", Type: "A", Data: "203.0.113.5", TTL: 300 * time.Second},
	}}
	rt, db := newTestRouterWithDNSRecordManager(t, mgr)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	body := `{"original":{"name":"www","type":"A","value":"203.0.113.5","ttl_seconds":300},"record":{"name":"www","type":"A","value":"203.0.113.9","ttl_seconds":300}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/dns-records", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(mgr.recs) != 1 || mgr.recs[0].RR().Data != "203.0.113.9" {
		t.Fatalf("mgr.recs = %+v, want one record with data 203.0.113.9", mgr.recs)
	}
}

func TestHandleDeleteDNSRecord_Idempotent(t *testing.T) {
	mgr := &fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "www", Type: "A", Data: "203.0.113.5", TTL: 300 * time.Second},
	}}
	rt, db := newTestRouterWithDNSRecordManager(t, mgr)
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"www","type":"A","value":"203.0.113.5"}`
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/dns-records", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("delete #%d: status = %d, want %d; body = %s", i, rec.Code, http.StatusOK, rec.Body.String())
		}
	}
	if len(mgr.recs) != 0 {
		t.Fatalf("mgr.recs = %+v, want empty after deleting the only record", mgr.recs)
	}
}

func TestDNSZoneApexGuess(t *testing.T) {
	cases := map[string]string{
		"app.example.com":     "example.com.",
		"a.b.app.example.com": "example.com.",
		"example.com":         "example.com.",
	}
	for domain, want := range cases {
		if got := dnsZoneApexGuess(domain); got != want {
			t.Errorf("dnsZoneApexGuess(%q) = %q, want %q", domain, got, want)
		}
	}
}
