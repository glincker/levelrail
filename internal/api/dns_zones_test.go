package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/miekg/dns"
)

type fakeZoneProvider struct {
	mu      sync.Mutex
	name    string
	caps    dnszones.Capabilities
	zones   []dnszones.Zone
	sets    map[string][]dnszones.RecordSet
	deleted []string
}

func newFakeZoneProvider() *fakeZoneProvider {
	return &fakeZoneProvider{
		name: "cloudflare", caps: dnszones.Capabilities{Proxied: true, ApexCNAME: true},
		zones: []dnszones.Zone{{ID: "z1", Name: "example.com", Provider: "cloudflare", NameServers: []string{"ana.ns.cloudflare.com", "bob.ns.cloudflare.com"}}},
		sets: map[string][]dnszones.RecordSet{"z1": {
			{Name: "@", Type: "NS", TTL: 86400, Values: []string{"ana.ns.cloudflare.com"}},
			{Name: "www", Type: "A", TTL: 300, Values: []string{"203.0.113.1"}},
		}},
	}
}

func (f *fakeZoneProvider) Name() string                        { return f.name }
func (f *fakeZoneProvider) Capabilities() dnszones.Capabilities { return f.caps }
func (f *fakeZoneProvider) ListZones(context.Context) ([]dnszones.Zone, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.zones), nil
}

func (f *fakeZoneProvider) GetZone(_ context.Context, id string) (dnszones.Zone, error) {
	z, err := dnszones.FindZone(f.zones, id)
	return z, err
}

func (f *fakeZoneProvider) CreateZone(_ context.Context, name string) (dnszones.Zone, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	z := dnszones.Zone{ID: "z" + name, Name: name, NameServers: []string{"ana.ns.cloudflare.com"}}
	f.zones = append(f.zones, z)
	return z, nil
}

func (f *fakeZoneProvider) DeleteZone(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeZoneProvider) ListRecordSets(_ context.Context, z dnszones.Zone) ([]dnszones.RecordSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sets[z.ID]), nil
}

func (f *fakeZoneProvider) UpsertRecordSet(_ context.Context, z dnszones.Zone, rs dnszones.RecordSet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets[z.ID] = append(slices.DeleteFunc(f.sets[z.ID], func(e dnszones.RecordSet) bool { return e.Key() == rs.Key() }), rs)
	return nil
}

func (f *fakeZoneProvider) DeleteRecordSet(_ context.Context, z dnszones.Zone, k dnszones.Key) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets[z.ID] = slices.DeleteFunc(f.sets[z.ID], func(e dnszones.RecordSet) bool { return e.Key() == k })
	return nil
}

type fakeAPIQuerier struct{ answers map[string][]string }

func (f fakeAPIQuerier) Query(_ context.Context, server, name string, t uint16) dnszones.Answer {
	return dnszones.Answer{Server: server, Values: f.answers[dnszones.NormalizeDomain(name)+"|"+dns.TypeToString[t]], TTL: 120}
}

func newDNSZoneTestRouter(t *testing.T, p *fakeZoneProvider) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.dnsZoneProviders = func(context.Context) (map[string]dnszones.Provider, error) {
		if p == nil {
			return map[string]dnszones.Provider{}, nil
		}
		return map[string]dnszones.Provider{p.name: p}, nil
	}
	rt.dnsQuerier = fakeAPIQuerier{answers: map[string][]string{
		"example.com|NS":     {"ana.ns.cloudflare.com", "bob.ns.cloudflare.com"},
		"www.example.com|A":  {"203.0.113.1"},
		"mail.example.com|A": {"198.51.100.7"},
	}}
	return rt, db, loginTestSession(t, rt, db)
}

func serveDNS(t *testing.T, rt *Router, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDNSZones_NoProviderIsEmptyState(t *testing.T) {
	rt, _, cookie := newDNSZoneTestRouter(t, nil)
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"none"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/example.com/records", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("records without a provider = %d, want 501", rec.Code)
	}
}

func TestDNSZones_ListOverviewAndDelegation(t *testing.T) {
	rt, _, cookie := newDNSZoneTestRouter(t, newFakeZoneProvider())
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones", ""))
	var list dnsZonesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Zones) != 1 || list.Provider != "cloudflare" {
		t.Fatalf("list = %s", rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/example.com", ""))
	var ov dnsZoneOverview
	if err := json.Unmarshal(rec.Body.Bytes(), &ov); err != nil || ov.Counts["A"] != 1 || ov.Total != 2 {
		t.Fatalf("overview = %s", rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/z1/delegation", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"delegated"`) {
		t.Errorf("delegation = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/nope.org/records", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown zone = %d", rec.Code)
	}
}

func TestDNSZones_RecordCRUDConflictsAndAudit(t *testing.T) {
	p := newFakeZoneProvider()
	rt, db, cookie := newDNSZoneTestRouter(t, p)
	base := "/api/v1/dns/zones/example.com/records"

	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, base, `{"name":"www","type":"CNAME","values":["x.net"]}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), dnszones.IssueCNAMEExclusive) {
		t.Fatalf("cname over A = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, base, `{"name":"api","type":"A","values":["1.2.3.4"],"weight":5,"routing":"weighted","set_identifier":"a"}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Route53") {
		t.Fatalf("weighted on cloudflare = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, base, `{"name":"api","type":"A","values":["1.2.3.4","1.2.3.5"],"proxied":true}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, base, `{"name":"@","type":"CNAME","values":["x.net"]}`))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), dnszones.IssueApexFlattened) {
		t.Fatalf("apex cname on cloudflare should warn, got %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPut, base, `{"original":{"name":"api","type":"A"},"record":{"name":"api2","type":"A","values":["9.9.9.9"]}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, base+"?q=9.9.9", ""))
	var list dnsRecordsListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Records) != 1 || list.Records[0].Name != "api2" {
		t.Fatalf("search after rename = %s", rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodDelete, base+"?name=@&type=NS", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("deleting apex NS = %d", rec.Code)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodDelete, base+"?name=api2&type=A", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}

	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: "dns_record"})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range entries {
		actions = append(actions, e.Action)
		if strings.Contains(e.Path, "token") {
			t.Errorf("audit path leaks a credential: %s", e.Path)
		}
	}
	for _, want := range []string{auditDNSRecordCreate, auditDNSRecordUpdate, auditDNSRecordDelete} {
		if !slices.Contains(actions, want) {
			t.Errorf("missing audit action %s in %v", want, actions)
		}
	}
}

func TestDNSZones_ReadTokenCannotWrite(t *testing.T) {
	rt, db, _ := newDNSZoneTestRouter(t, newFakeZoneProvider())
	const plaintext = "read-scoped-token" //nolint:gosec // test fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_read", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, "/api/v1/dns/zones", "", http.StatusOK},
		{http.MethodGet, "/api/v1/dns/zones/example.com/records", "", http.StatusOK},
		{http.MethodGet, "/api/v1/dns/templates", "", http.StatusOK},
		{http.MethodPost, "/api/v1/dns/zones", `{"name":"new.com"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/dns/zones/example.com/records", `{"name":"a","type":"A","values":["1.1.1.1"]}`, http.StatusForbidden},
		{http.MethodDelete, "/api/v1/dns/zones/example.com", `{"confirm":"example.com"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/dns/zones/example.com/records/import", `{"format":"bind","content":""}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		if rec := serveDNS(t, rt, req); rec.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestDNSZones_CreateAndDeleteZone(t *testing.T) {
	p := newFakeZoneProvider()
	rt, _, cookie := newDNSZoneTestRouter(t, p)
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"bad name", http.MethodPost, "/api/v1/dns/zones", `{"name":"com"}`, http.StatusBadRequest},
		{"exists", http.MethodPost, "/api/v1/dns/zones", `{"name":"example.com"}`, http.StatusConflict},
		{"create", http.MethodPost, "/api/v1/dns/zones", `{"name":"new.dev"}`, http.StatusCreated},
		{"no confirm", http.MethodDelete, "/api/v1/dns/zones/example.com", `{}`, http.StatusBadRequest},
		{"wrong confirm", http.MethodDelete, "/api/v1/dns/zones/example.com", `{"confirm":"other.com"}`, http.StatusBadRequest},
		{"has records", http.MethodDelete, "/api/v1/dns/zones/example.com", `{"confirm":"example.com"}`, http.StatusConflict},
		{"forced", http.MethodDelete, "/api/v1/dns/zones/example.com", `{"confirm":"example.com","force":true}`, http.StatusOK},
		{"empty zone needs no force", http.MethodDelete, "/api/v1/dns/zones/new.dev", `{"confirm":"new.dev"}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := serveDNS(t, rt, authedRequest(t, cookie, tt.method, tt.path, tt.body)); rec.Code != tt.want {
				t.Errorf("= %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
		})
	}
	if len(p.deleted) != 2 {
		t.Errorf("deleted = %v", p.deleted)
	}
}

func TestDNSZones_ImportExportTemplateDiscover(t *testing.T) {
	p := newFakeZoneProvider()
	rt, _, cookie := newDNSZoneTestRouter(t, p)
	imp := "/api/v1/dns/zones/example.com/records/import"
	zone := "www 300 IN A 203.0.113.9\nmail 300 IN A 198.51.100.7\n@ 300 IN MX 10 mail.example.com.\n"
	body, _ := json.Marshal(map[string]any{"format": "bind", "content": zone})
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, imp, string(body)))
	var res importDNSRecordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Plan.Summary["create"] != 2 || res.Plan.Summary["update"] != 1 || res.Applied != 0 {
		t.Fatalf("preview = %s", rec.Body)
	}
	body, _ = json.Marshal(map[string]any{"format": "bind", "content": zone, "apply": true, "replace": true})
	if rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, imp, string(body))); rec.Code != http.StatusBadRequest {
		t.Fatalf("replace without confirm = %d", rec.Code)
	}
	body, _ = json.Marshal(map[string]any{"format": "bind", "content": zone, "apply": true})
	if rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, imp, string(body))); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/example.com/records/export?format=bind", ""))
	if !strings.Contains(rec.Body.String(), "www\t300\tIN\tA\t203.0.113.9") || strings.Contains(rec.Body.String(), "\tNS\t") {
		t.Errorf("export = %s", rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/dns/zones/example.com/templates/caa-letsencrypt", `{"apply":true}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"applied":1`) {
		t.Errorf("template = %d %s", rec.Code, rec.Body)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/dns/zones/example.com/templates/email", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("template missing param = %d", rec.Code)
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/check?name=www.example.com&type=A", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"agree":true`) || !strings.Contains(rec.Body.String(), "authoritative") {
		t.Errorf("check = %d %s", rec.Code, rec.Body)
	}
}

func TestWildcardDomains(t *testing.T) {
	rt, db, cookie := newDNSZoneTestRouter(t, newFakeZoneProvider())
	seedAppWithDomains(t, db, "app.example.com")
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"add":["*.apps.example.com"]}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), wildcardNeedsDNS01) {
		t.Fatalf("wildcard without DNS-01 = %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{"*.com", "*.*.example.com", "a.*.example.com"} {
		body := `{"add":["` + bad + `"]}`
		if rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/apps/web/domains", body)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", bad, rec.Code)
		}
	}
	if rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"add":["b.example.com"]}`)); rec.Code != http.StatusOK {
		t.Errorf("plain domain = %d %s", rec.Code, rec.Body)
	}

	if h := wildcardProbeHost("*.apps.example.com"); !strings.HasSuffix(h, ".apps.example.com") || strings.Contains(h, "*") {
		t.Errorf("probe host = %q", h)
	}
	if h := wildcardProbeHost("x.example.com"); h != "x.example.com" {
		t.Errorf("exact probe host = %q", h)
	}
}

func TestCreateWildcardRecords(t *testing.T) {
	p := newFakeZoneProvider()
	rt, _, _ := newDNSZoneTestRouter(t, p)
	rt.publicHost = "203.0.113.50"
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/web/domains", nil)
	got := rt.createWildcardRecords(context.Background(), req, []string{"*.apps.example.com", "*.unmanaged.org"})
	if !slices.Equal(got, []string{"*.apps.example.com"}) {
		t.Fatalf("created = %v", got)
	}
	sets, _ := p.ListRecordSets(context.Background(), dnszones.Zone{ID: "z1"})
	i := slices.IndexFunc(sets, func(s dnszones.RecordSet) bool { return s.Name == "*.apps" && s.Type == "A" })
	if i < 0 || sets[i].Values[0] != "203.0.113.50" {
		t.Fatalf("sets = %+v", sets)
	}
	p.sets["z1"][i].Values = []string{"198.51.100.1"}
	if got := rt.createWildcardRecords(context.Background(), req, []string{"*.apps.example.com"}); len(got) != 0 {
		t.Error("an existing wildcard set must not be overwritten")
	}
}
