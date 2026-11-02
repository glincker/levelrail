package api

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
	"github.com/miekg/dns"
)

// staleCacheExchange makes recursive queries (public resolvers, this host's
// resolver) answer with the old proxy address while the zone's servers
// already return the new one.
func staleCacheExchange(_ context.Context, server, name string, qt uint16, recurse bool) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qt)
	switch {
	case qt == dns.TypeNS && name == "example.com":
		m.Answer = append(m.Answer, &dns.NS{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 60}, Ns: "ns1.example.com."})
	case qt == dns.TypeA && name == "ns1.example.com":
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: "ns1.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("1.2.3.4")})
	case qt == dns.TypeA && name == "app.example.com":
		ip, ttl := "72.61.8.70", uint32(60)
		if recurse && server == "10.0.0.1:53" {
			ip, ttl = "104.21.17.53", 900
		}
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: "app.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: net.ParseIP(ip)})
	}
	return m, nil
}

func TestDomainCheckConnectedWhenAuthoritativeIsRightButLocalIsStale(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(_ context.Context, host string) ([]string, error) {
		if host == "72.61.8.70" {
			return []string{"72.61.8.70"}, nil
		}
		return []string{"104.21.17.53"}, nil
	}
	rt.publicLookup = func(context.Context, string) []resolverResult { return nil }
	rt.dnsFlushCommand = func() string { return "resolvectl flush-caches" }
	rt.dnsInspector = &dnsinspect.Inspector{Public: []string{"10.0.0.1:53"}, LocalServers: []string{"10.0.0.1:53"}, AllowPrivate: true, Exchange: staleCacheExchange}

	got := rt.runDomainCheckOpts(context.Background(), "app.example.com", "72.61.8.70", false, true)
	if got.Status != domainCheckStatusConnected || got.Verdict != "dns_ready" || !got.LocalStale {
		t.Fatalf("status=%s verdict=%s stale=%v, want connected/dns_ready/stale", got.Status, got.Verdict, got.LocalStale)
	}
	if got.RetryAt == "" || got.FlushCommand != "resolvectl flush-caches" || len(got.Authoritative) != 1 || got.Local[0].TTL != 900 {
		t.Errorf("retry=%q flush=%q auth=%+v local=%+v", got.RetryAt, got.FlushCommand, got.Authoritative, got.Local)
	}
	var cf bool
	for _, h := range got.Hints {
		cf = cf || h.Code == "was_cloudflare_proxy"
	}
	if !cf {
		t.Errorf("hints = %+v, want the Cloudflare explanation", got.Hints)
	}
}

func TestDomainCheckFailingLookupKeepsOldBehaviour(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") }
	rt.publicLookup = func(context.Context, string) []resolverResult { return nil }
	got := rt.runDomainCheckOpts(context.Background(), "app.example.com", "72.61.8.70", false, true)
	if got.Verdict != "" || got.Status != domainCheckStatusNotResolving {
		t.Errorf("with public resolvers off, verdict=%q status=%s", got.Verdict, got.Status)
	}
}
