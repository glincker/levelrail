package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

type emptyStatusResolver struct{}

var errStatusNX = errors.New("no such record")

func (emptyStatusResolver) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errStatusNX
}
func (emptyStatusResolver) LookupCNAME(context.Context, string) (string, error) {
	return "", errStatusNX
}
func (emptyStatusResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return nil, errStatusNX
}
func (emptyStatusResolver) LookupHost(context.Context, string) ([]string, error) {
	return nil, errStatusNX
}
func (emptyStatusResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errStatusNX
}

func TestDomainDNSStatus_TeamScopingAndSource(t *testing.T) {
	t.Setenv(dnsstatus.EnvCacheSeconds, "0")
	f := newTeamFixture(t)
	f.rt.domains = stubServiceDomains{{Domain: "www.zone-a.test", ServiceName: appA}, {Domain: "www.zone-b.test", ServiceName: appB}}
	f.rt.mail.nsResolver = stubNS{"zone-a.test": {"ana.ns.cloudflare.com"}, "zone-b.test": {"bob.ns.cloudflare.com"}}
	f.rt.mail.statusResolver = emptyStatusResolver{}
	if err := f.db.UpdateIngressSettings(context.Background(), store.IngressSettings{DNSCNAMETarget: "edge.example.net"}); err != nil {
		t.Fatal(err)
	}
	p := newFakeZoneProvider()
	p.zones = []dnszones.Zone{{ID: "zone-a-id", Name: "zone-a.test", Provider: "cloudflare"}, {ID: "zone-b-id", Name: "zone-b.test", Provider: "cloudflare"}}
	p.sets = map[string][]dnszones.RecordSet{"zone-b-id": {{Name: "www", Type: "CNAME", TTL: 1, Values: []string{"Edge.Example.net."}, Proxied: true}}}
	f.rt.dnsZoneProviders = func(context.Context) (map[string]dnszones.Provider, error) {
		return map[string]dnszones.Provider{p.name: p}, nil
	}
	get := func(c *http.Cookie, query string, want int) dnsStatusResponse {
		t.Helper()
		code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/dns-status?"+query, "", withCookie(c))
		if code != want {
			t.Fatalf("GET %s = %d %s", query, code, body)
		}
		for _, leak := range []string{"token", "secret", "api_key", "zone-b-id"} {
			if strings.Contains(strings.ToLower(body), leak) {
				t.Fatalf("response mentions %q: %s", leak, body)
			}
		}
		var out dnsStatusResponse
		_ = json.Unmarshal([]byte(body), &out)
		return out
	}
	admin := loginTestSession(t, f.rt, f.db)
	got := get(admin, "domain=www.zone-b.test", http.StatusOK)
	if got.ProviderSource != dnsstatus.SourceProvider || len(got.Records) != 1 || got.Records[0].Status != dnsstatus.Live || got.Records[0].Found[0].Proxied == nil {
		t.Fatalf("admin view = %+v", got)
	}
	member := f.cookies[store.TeamRoleViewer]
	got = get(member, "domain=www.zone-b.test", http.StatusOK)
	if got.ProviderSource != dnsstatus.SourcePublicDNS || got.Records[0].Status != dnsstatus.Waiting || len(got.Records[0].Found) != 0 {
		t.Fatalf("team A must only get public DNS for team B's zone: %+v", got)
	}
	get(member, "domain=www.zone-b.test&app="+appB, http.StatusNotFound)
	if got := get(member, "domain=www.zone-a.test&app="+appA, http.StatusOK); got.ProviderSource != dnsstatus.SourceProvider || got.Counts.Waiting != 1 {
		t.Fatalf("team A must see its own zone: %+v", got)
	}
	get(admin, "domain=not%20a%20domain", http.StatusBadRequest)
}

func TestMailDNSStatus_DemoZoneWaitingThenLive(t *testing.T) {
	f := newTeamFixture(t)
	f.rt.mail.demo = &mailDemo{engine: demo.NewEngine(), zone: demo.NewZone()}
	status := func() map[string]dnsstatus.Record {
		t.Helper()
		rec := httptest.NewRecorder()
		f.rt.handleMailDNSStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/mail/dns/status?domain="+demo.DefaultDomain, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		var out dnsStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		m := map[string]dnsstatus.Record{}
		for _, r := range out.Records {
			m[r.ID] = r
		}
		return m
	}
	before := status()
	if before["spf-txt"].Status != dnsstatus.Waiting || before["dmarc-txt"].Status != dnsstatus.Waiting {
		t.Fatalf("before apply = %+v", before)
	}
	spec := mailrecords.Spec{Domain: demo.DefaultDomain}
	f.rt.mailDemoFillSpec(&spec)
	desired, _, err := mailrecords.Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	z := f.rt.mail.demo.zone
	for _, d := range desired {
		if err := z.UpsertRecordSet(context.Background(), z.ZoneFor(demo.DefaultDomain), d.Set); err != nil {
			t.Fatal(err)
		}
	}
	after := status()
	if after["spf-txt"].Status != dnsstatus.Live || after["dmarc-txt"].Status != dnsstatus.Live {
		t.Fatalf("after apply = %+v", after)
	}
}
