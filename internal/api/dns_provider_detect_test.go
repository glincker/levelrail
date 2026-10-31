package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

type stubNS map[string][]string

func (s stubNS) LookupNS(_ context.Context, name string) ([]*net.NS, error) {
	var out []*net.NS
	for _, h := range s[name] {
		out = append(out, &net.NS{Host: h})
	}
	if len(out) == 0 {
		return nil, errors.New("no such host")
	}
	return out, nil
}

func (stubNS) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errors.New("no such host")
}

type stubServiceDomains []store.ServiceDomain

func (s stubServiceDomains) ListServiceDomains(context.Context) ([]store.ServiceDomain, error) {
	return s, nil
}

const testCFAccount = "0123456789abcdef0123456789abcdef"

func TestDomainDNSProvider_TeamScopingAndDashboardURL(t *testing.T) {
	f := newTeamFixture(t)
	f.rt.domains = stubServiceDomains{{Domain: "www.zone-a.test", ServiceName: appA}, {Domain: "www.zone-b.test", ServiceName: appB}}
	f.rt.mail.nsResolver = stubNS{"zone-a.test": {"ana.ns.cloudflare.com"}, "zone-b.test": {"bob.ns.cloudflare.com"}}
	p := newFakeZoneProvider()
	p.zones = []dnszones.Zone{
		{ID: "zone-a-id", Name: "zone-a.test", Provider: "cloudflare", AccountID: testCFAccount},
		{ID: "zone-b-secretish-id", Name: "zone-b.test", Provider: "cloudflare", AccountID: testCFAccount},
	}
	f.rt.dnsZoneProviders = func(context.Context) (map[string]dnszones.Provider, error) {
		return map[string]dnszones.Provider{p.name: p}, nil
	}
	get := func(c *http.Cookie, domain string) dnsProviderResponse {
		t.Helper()
		code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/dns-provider?domain="+domain, "", withCookie(c))
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", domain, code, body)
		}
		for _, leak := range []string{"token", "secret_access", "api_key"} {
			if strings.Contains(strings.ToLower(body), leak) {
				t.Fatalf("response mentions %q: %s", leak, body)
			}
		}
		var out dnsProviderResponse
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	member := f.cookies[store.TeamRoleViewer]
	if got := get(member, "mail.zone-b.test"); got.Connected || got.ZoneID != "" || got.DashboardURL != "" || got.Provider != mailrecords.HostCloudflare {
		t.Fatalf("team A learned team B's zone: %+v", got)
	}
	if got := get(member, "zone-a.test"); !got.Connected || got.ZoneID != "zone-a-id" || !got.CanIssueCertificates {
		t.Fatalf("team A must see its own zone: %+v", got)
	}
	admin := loginTestSession(t, f.rt, f.db)
	got := get(admin, "www.zone-b.test")
	if !got.Connected || got.DashboardURL != "https://dash.cloudflare.com/"+testCFAccount+"/zone-b.test/dns/records" {
		t.Fatalf("admin view = %+v", got)
	}
	p.zones[1].AccountID = ""
	f.rt.mail.nsCache = mailNSCache{}
	if got := get(admin, "zone-b.test"); got.DashboardURL != "" {
		t.Fatalf("no account id must mean no URL: %+v", got)
	}
	p.zones = p.zones[:1]
	if got := get(admin, "zone-b.test"); got.Connected || !strings.Contains(got.Hint, "cannot see the zone zone-b.test") {
		t.Fatalf("invisible zone must say so: %+v", got)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/dns-provider?domain=com", "", withCookie(admin)); code != http.StatusBadRequest {
		t.Fatalf("public suffix = %d, want 400", code)
	}
}

func TestMergeMailDomainsDedupes(t *testing.T) {
	got := mergeMailDomains([]mailDNSDomain{
		{Domain: "Example.com.", Source: mailDomainSourceInstall},
		{Domain: "example.com", Source: mailDomainSourceZone},
		{Domain: "app.example.com", Source: mailDomainSourceApp},
		{Domain: "example.com", Source: mailDomainSourceMail},
		{Domain: "not a domain", Source: mailDomainSourceApp},
	})
	if len(got) != 2 || got[0].Domain != "example.com" || got[0].Source != mailDomainSourceInstall || strings.Join(got[0].Sources, ",") != "install,zone,mail" {
		t.Fatalf("merged = %+v", got)
	}
}

func TestMailLedgerRoundTrip(t *testing.T) {
	recs := []mailrecords.AppliedRecord{{Name: "mail", Type: "A", Action: "add", IDs: []string{"id1", "id2"}}, {Name: "@", Type: "MX", Action: "add", IDs: []string{}}}
	path := dnsZoneAuditPrefix + "z1#" + mailLedgerSubject("example.com", "cloudflare", 2, recs)
	if strings.Contains(path, "203.0.113") {
		t.Fatal("the ledger must not carry record values")
	}
	e, ok := parseMailLedger(path, "example.com")
	if !ok || e.ZoneID != "z1" || e.Provider != "cloudflare" || len(e.Records) != 2 || strings.Join(e.Records[0].IDs, ",") != "id1,id2" || len(e.Records[1].IDs) != 0 {
		t.Fatalf("parsed = %+v %v", e, ok)
	}
	if _, ok := parseMailLedger(path, "other.com"); ok {
		t.Fatal("a row of another domain must not match")
	}
}
