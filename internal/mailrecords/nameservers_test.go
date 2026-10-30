package mailrecords

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

type fakeNS struct {
	ns  map[string][]string
	txt map[string][]string
}

func (f fakeNS) LookupNS(_ context.Context, name string) ([]*net.NS, error) {
	var out []*net.NS
	for _, h := range f.ns[name] {
		out = append(out, &net.NS{Host: h + "."})
	}
	if len(out) == 0 {
		return nil, errors.New("no such host")
	}
	return out, nil
}

func (f fakeNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, errors.New("no such host")
}

func TestClassifyNameservers(t *testing.T) {
	cases := map[string][]string{
		HostCloudflare:   {"ana.ns.cloudflare.com", "bob.ns.cloudflare.com."},
		HostRoute53:      {"ns-1.awsdns-01.org", "ns-2.awsdns-02.co.uk"},
		HostDigitalOcean: {"ns1.digitalocean.com"},
		HostGoogleCloud:  {"ns-cloud-a1.googledomains.com"},
		HostAzure:        {"ns1-01.azure-dns.com", "ns2-01.azure-dns.net"},
		HostHetzner:      {"hydrogen.ns.hetzner.com"},
		HostNamecheap:    {"dns1.registrar-servers.com"},
		HostGoDaddy:      {"ns01.domaincontrol.com"},
		HostManual:       {"ns1.example.net"},
	}
	for want, ns := range cases {
		if got := ClassifyNameservers(ns); got != want {
			t.Errorf("%v = %s, want %s", ns, got, want)
		}
	}
	if got := ClassifyNameservers([]string{"ana.ns.cloudflare.com", "ns1.digitalocean.com"}); got != HostManual {
		t.Errorf("mixed hosts = %s, want manual", got)
	}
	if got := ClassifyNameservers(nil); got != HostManual {
		t.Errorf("empty = %s, want manual", got)
	}
}

func TestDetectWalksUpToTheZoneAndDiscoversAtTheApex(t *testing.T) {
	r := fakeNS{
		ns:  map[string][]string{"example.co.uk": {"ana.ns.cloudflare.com", "bob.ns.cloudflare.com"}},
		txt: map[string][]string{"_domainconnect.example.co.uk": {"api.cloudflare.com/client/v4/dns/domainconnect"}},
	}
	d, err := Detect(context.Background(), r, "mail.eu.example.co.uk")
	if err != nil {
		t.Fatal(err)
	}
	if d.Apex != "example.co.uk" || d.Zone != "example.co.uk" || d.Provider != HostCloudflare || len(d.Nameservers) != 2 {
		t.Fatalf("detection = %+v", d)
	}
	if !d.DomainConnect.Supported || d.DomainConnect.Host != "api.cloudflare.com" || d.DomainConnect.Path != "/client/v4/dns/domainconnect" {
		t.Fatalf("domain connect = %+v", d.DomainConnect)
	}
	// A delegated subdomain reports its own host; discovery falls back from the apex to that zone.
	r = fakeNS{
		ns:  map[string][]string{"eu.example.com": {"ns-1.awsdns-01.org"}, "example.com": {"ana.ns.cloudflare.com"}},
		txt: map[string][]string{"_domainconnect.eu.example.com": {"dc.example.net"}},
	}
	d, err = Detect(context.Background(), r, "mail.eu.example.com")
	if err != nil || d.Zone != "eu.example.com" || d.Provider != HostRoute53 || d.DomainConnect.Host != "dc.example.net" {
		t.Fatalf("delegated = %+v err %v", d, err)
	}
	if _, err := Detect(context.Background(), fakeNS{}, "nothing.example.org"); err == nil {
		t.Fatal("no NS anywhere must be an error")
	}
	if _, err := Detect(context.Background(), fakeNS{}, "com"); err == nil {
		t.Fatal("a public suffix is not a registrable domain")
	}
}

func TestParseDomainConnect(t *testing.T) {
	if dc, ok := ParseDomainConnect([]string{"domainconnect.godaddy.com"}); !ok || dc.Host != "domainconnect.godaddy.com" || dc.Path != "" {
		t.Fatalf("valid host = %+v %v", dc, ok)
	}
	for _, bad := range [][]string{nil, {""}, {"v=spf1 -all"}, {"http://evil.example/x"}, {"203.0.113.9"}, {"host.example/../x"}, {"localhost"}, {"a b.example"}} {
		if dc, ok := ParseDomainConnect(bad); ok {
			t.Errorf("%q accepted as %+v", bad, dc)
		}
	}
}

func TestPTRHint(t *testing.T) {
	if h := PTRHint(HostFromPTR([]string{"static.7.113.0.203.clients.your-server.de."})); !strings.Contains(h, "Hetzner") {
		t.Errorf("hetzner hint = %s", h)
	}
	if h := PTRHint(HostFromPTR([]string{"mail.example.com"})); !strings.Contains(h, "hosting provider") || strings.Contains(h, "here") {
		t.Errorf("generic hint = %s", h)
	}
}

func TestPlanRefusesProxiedMailHost(t *testing.T) {
	p := &memProvider{sets: []dnszones.RecordSet{{Name: "mail", Type: "A", TTL: 300, Values: []string{"203.0.113.7"}, Proxied: true}}}
	spec := Spec{Domain: "example.com", IPv4: "203.0.113.7"}
	res, err := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked || len(res.Shadowed) != 1 || !strings.Contains(res.Shadowed[0], "mail.example.com A is proxied") {
		t.Fatalf("want a proxied refusal, got %+v", res)
	}
	if _, _, err := Apply(context.Background(), p, zone, spec, PlanOptions{}, res.Hash); !errors.Is(err, ErrBlocked) || len(p.ops) != 0 {
		t.Fatalf("apply must refuse and write nothing: %v %v", err, p.ops)
	}
}

type idProvider struct{ memProvider }

func (p *idProvider) RecordIDs(_ context.Context, _ dnszones.Zone, k dnszones.Key, values []string) ([]string, error) {
	var ids []string
	for i := range values {
		ids = append(ids, k.Name+"-"+k.Type+"-"+string(rune('a'+i)))
	}
	return ids, nil
}

func TestApplyCreatesDNSOnlyTaggedRecordsAndListsIDs(t *testing.T) {
	p := &idProvider{}
	spec := Spec{Domain: "example.com", IPv4: "203.0.113.7", Autoconfig: true}
	res, err := Plan(context.Background(), p, zone, spec, PlanOptions{})
	if err != nil || res.Blocked {
		t.Fatalf("plan %+v %v", res, err)
	}
	if _, _, err := Apply(context.Background(), p, zone, spec, PlanOptions{}, res.Hash); err != nil {
		t.Fatal(err)
	}
	for _, s := range p.sets {
		if s.Proxied || s.Comment != ManagedComment {
			t.Errorf("%s %s proxied=%t comment=%q", s.Name, s.Type, s.Proxied, s.Comment)
		}
	}
	recs := AppliedRecords(context.Background(), p, zone, res)
	if len(recs) == 0 || len(recs[0].IDs) == 0 {
		t.Fatalf("applied records = %+v", recs)
	}
}
