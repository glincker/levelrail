package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeRegistrar struct{ ok bool }

func (f fakeRegistrar) Registrar(context.Context, string) (dnszones.RegistrarHint, bool) {
	if !f.ok {
		return dnszones.RegistrarHint{}, false
	}
	return dnszones.RegistrarHint{Name: "Example Registrar", URL: "https://registrar.example"}, true
}

func assertNoSecrets(t *testing.T, body string) {
	t.Helper()
	for _, leak := range []string{"token", "secret", "api_key"} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Fatalf("response mentions %q: %s", leak, body)
		}
	}
}

func TestDNSZones_ListDelegationStates(t *testing.T) {
	p := newFakeZoneProvider()
	p.zones = append(p.zones,
		dnszones.Zone{ID: "z2", Name: "example.net", Provider: "cloudflare", Status: "pending", NameServers: []string{"cleo.ns.cloudflare.com"}},
		dnszones.Zone{ID: "z3", Name: "example.org", Provider: "cloudflare", NameServers: []string{"ana.ns.cloudflare.com"}, Private: true},
		dnszones.Zone{ID: "z4", Name: "example.info", Provider: "cloudflare", NameServers: []string{"ana.ns.cloudflare.com"}},
	)
	rt, _, cookie := newDNSZoneTestRouter(t, p)
	rt.dnsQuerier = fakeAPIQuerier{answers: map[string][]string{
		"example.com|NS":  {"ANA.ns.cloudflare.com.", "bob.ns.cloudflare.com."},
		"example.net|NS":  {"ns1.registrar.example."},
		"example.info|NS": {"ana.ns.cloudflare.com", "ns1.registrar.example"},
	}}
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones", ""))
	assertNoSecrets(t, rec.Body.String())
	var list dnsZonesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Zones) != 4 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	got := map[string]dnsZoneView{}
	for _, z := range list.Zones {
		got[z.Name] = z
	}
	if z := got["example.com"]; z.Delegated != dnszones.DelegatedYes || len(z.Nameservers) != 2 {
		t.Errorf("example.com = %+v", z)
	}
	if z := got["example.net"]; z.Delegated != dnszones.DelegatedNo || strings.Join(z.CurrentNameservers, ",") != "ns1.registrar.example" {
		t.Errorf("example.net = %+v", z)
	}
	if z := got["example.info"]; z.Delegated != dnszones.DelegatedPartial {
		t.Errorf("example.info = %+v", z)
	}
	if z := got["example.org"]; z.Delegated != dnszones.DelegatedUnknown || z.DelegationReason != dnszones.DelegationReasonPrivateZone {
		t.Errorf("private zone = %+v", z)
	}
	if !strings.Contains(rec.Body.String(), `"capabilities_by_provider":{"cloudflare"`) {
		t.Errorf("missing capabilities_by_provider: %s", rec.Body)
	}
}

func TestDNSZones_DetailWithRegistrarHint(t *testing.T) {
	rt, _, cookie := newDNSZoneTestRouter(t, newFakeZoneProvider())
	rt.dnsNS.registrar = fakeRegistrar{ok: true}
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/zones/cloudflare/z1", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"registrar_hint":{"name":"Example Registrar"`) || !strings.Contains(rec.Body.String(), `"delegated":"yes"`) {
		t.Fatalf("detail = %d %s", rec.Code, rec.Body)
	}
	rt2, _, cookie2 := newDNSZoneTestRouter(t, newFakeZoneProvider())
	rt2.dnsNS.registrar = fakeRegistrar{}
	rec = serveDNS(t, rt2, authedRequest(t, cookie2, http.MethodGet, "/api/v1/dns/zones/cloudflare/example.com", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "registrar_hint") {
		t.Fatalf("failed RDAP must only omit the hint: %d %s", rec.Code, rec.Body)
	}
	for _, path := range []string{"/api/v1/dns/zones/route53/z1", "/api/v1/dns/zones/cloudflare/nope"} {
		if rec := serveDNS(t, rt2, authedRequest(t, cookie2, http.MethodGet, path, "")); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
	// The literal sub routes still win over {provider}/{id}.
	if rec := serveDNS(t, rt2, authedRequest(t, cookie2, http.MethodGet, "/api/v1/dns/zones/z1/records", "")); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"records"`) {
		t.Errorf("records route shadowed: %d", rec.Code)
	}
}

func TestDNSZones_RecordsReadOnlyLedgerAndConfirm(t *testing.T) {
	p := newFakeZoneProvider()
	p.sets["z1"] = append(p.sets["z1"], dnszones.RecordSet{Name: "mail", Type: "A", TTL: 300, Values: []string{"198.51.100.7"}})
	rt, db, cookie := newDNSZoneTestRouter(t, p)
	id, err := store.NewAuditEntryID()
	if err != nil {
		t.Fatal(err)
	}
	subject := mailLedgerSubject("example.com", "cloudflare", 1, []mailrecords.AppliedRecord{{Name: "mail", Type: "A", Action: "add", IDs: []string{"r1"}}})
	if err := db.SaveAuditEntry(context.Background(), store.AuditEntry{ID: id, ActorType: "user", ActorID: "u", Method: http.MethodPost, Path: dnsZoneAuditPrefix + "z1#" + subject, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), Action: auditMailDNSApply}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/dns/zones/example.com/records"
	rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, base+"?limit=2", ""))
	var list dnsRecordsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Records) != 2 || list.NextCursor != "2" || list.Matched != 3 {
		t.Fatalf("page 1 = %s", rec.Body)
	}
	if !list.Records[0].ReadOnly || list.Records[0].Type != "NS" {
		t.Errorf("apex NS must be read only: %+v", list.Records[0])
	}
	rec = serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, base+"?search=mail&cursor=0", ""))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Records) != 1 || !list.Records[0].ManagedByLevelrail {
		t.Fatalf("ledger flag missing: %s", rec.Body)
	}
	if rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, base+"?cursor=99", "")); rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor = %d", rec.Code)
	}
	for _, c := range []struct {
		q    string
		code int
	}{
		{"?name=www&type=A", http.StatusBadRequest},
		{"?name=www&type=A&confirm=api", http.StatusBadRequest},
		{"?name=@&type=SOA&confirm=@", http.StatusBadRequest},
		{"?name=mail&type=A&confirm=mail", http.StatusConflict},
		{"?name=mail&type=A&confirm=mail.example.com&force=true", http.StatusOK},
		{"?name=www&type=A&confirm=www", http.StatusOK},
	} {
		rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodDelete, base+c.q, ""))
		if rec.Code != c.code {
			t.Errorf("DELETE %s = %d %s, want %d", c.q, rec.Code, rec.Body, c.code)
		}
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: auditDNSRecordDelete})
	if err != nil || len(entries) != 2 {
		t.Fatalf("delete audit rows = %d %v", len(entries), err)
	}
	for _, e := range entries {
		assertNoSecrets(t, e.Path)
	}
}

func TestDNSZones_RecordValidation(t *testing.T) {
	cases := []struct {
		rs      dnszones.RecordSet
		caps    dnszones.Capabilities
		problem string
	}{
		{dnszones.RecordSet{Name: "@", Type: "CNAME", Values: []string{"x.net"}}, dnszones.Capabilities{}, dnszones.IssueApexCNAME},
		{dnszones.RecordSet{Name: "@", Type: "CNAME", Values: []string{"x.net"}}, dnszones.Capabilities{ApexCNAME: true}, dnszones.IssueApexFlattened},
		{dnszones.RecordSet{Name: "www", Type: "TXT", Values: []string{"x"}}, dnszones.Capabilities{}, ""},
		{dnszones.RecordSet{Name: "www", Type: "CNAME", Values: []string{"x.net"}}, dnszones.Capabilities{}, dnszones.IssueCNAMEExclusive},
	}
	existing := []dnszones.RecordSet{{Name: "www", Type: "A", Values: []string{"203.0.113.1"}}}
	for _, c := range cases {
		issues := dnszones.Conflicts(existing, c.rs, c.caps, nil)
		found := c.problem == ""
		for _, is := range issues {
			found = found || is.Code == c.problem
		}
		if !found {
			t.Errorf("%s %s: issues %+v, want %s", c.rs.Name, c.rs.Type, issues, c.problem)
		}
	}
	if _, err := dnszones.Normalize(dnszones.RecordSet{Name: "@", Type: "MX", Values: []string{"mail.example.com"}}, "example.com", 300); err == nil {
		t.Error("MX without a priority must be rejected")
	}
	long := strings.Repeat("v", 600)
	rs, err := dnszones.Normalize(dnszones.RecordSet{Name: "@", Type: "TXT", Values: []string{long}}, "example.com", 300)
	if err != nil || rs.Values[0] != long {
		t.Errorf("long TXT must be accepted and split on write: %v", err)
	}
}

func TestDNSZones_TeamScopedListAndDelegationWarning(t *testing.T) {
	f := newTeamFixture(t)
	f.rt.domains = stubServiceDomains{{Domain: "www.zone-a.test", ServiceName: appA}, {Domain: "www.zone-b.test", ServiceName: appB}}
	f.rt.mail.nsResolver = stubNS{"zone-a.test": {"ns1.registrar.example"}, "zone-b.test": {"bob.ns.cloudflare.com"}}
	p := newFakeZoneProvider()
	p.zones = []dnszones.Zone{
		{ID: "zone-a-id", Name: "zone-a.test", Provider: "cloudflare", NameServers: []string{"ana.ns.cloudflare.com"}},
		{ID: "zone-b-id", Name: "zone-b.test", Provider: "cloudflare", NameServers: []string{"bob.ns.cloudflare.com"}},
	}
	f.rt.dnsZoneProviders = func(context.Context) (map[string]dnszones.Provider, error) {
		return map[string]dnszones.Provider{p.name: p}, nil
	}
	f.rt.dnsQuerier = fakeAPIQuerier{answers: map[string][]string{}}
	member := f.cookies[store.TeamRoleViewer]
	// Zone routes are instance wide, like every other zone route: a team member sees none of them.
	for _, path := range []string{"/api/v1/dns/zones", "/api/v1/dns/zones/cloudflare/zone-b-id", "/api/v1/dns/zones/zone-b.test/records"} {
		code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(member))
		if code == http.StatusOK || strings.Contains(body, "zone-b-id") {
			t.Fatalf("team member %s = %d %s", path, code, body)
		}
	}

	admin := loginTestSession(t, f.rt, f.db)
	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/dns-provider?domain=zone-a.test", "", withCookie(admin))
	var det dnsProviderResponse
	if err := json.Unmarshal([]byte(body), &det); err != nil || code != http.StatusOK {
		t.Fatalf("detect = %d %s", code, body)
	}
	if !det.Connected || det.Delegation == nil || det.Delegation.Delegated != dnszones.DelegatedNo || !det.DelegationWarning || !strings.Contains(det.DelegationReason, "ns1.registrar.example") {
		t.Fatalf("undelegated zone detection = %+v", det)
	}
	if det.CanIssueCertificates {
		t.Error("an undelegated zone cannot pass DNS-01")
	}

	code, body = teamDo(t, f.rt, http.MethodPost, "/api/v1/mail/dns/plan", `{"domain":"zone-a.test","mail_host":"mail.zone-a.test","ipv4":"203.0.113.5"}`, withCookie(admin))
	if code != http.StatusOK || !strings.Contains(body, `"delegation_warning":true`) || !strings.Contains(body, "still point to ns1.registrar.example") {
		t.Fatalf("mail plan = %d %s", code, body)
	}
	assertNoSecrets(t, body)
}
