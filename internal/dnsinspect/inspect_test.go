package dnsinspect

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// fakeDNS answers recursive queries from a stale cache and non-recursive
// ones from the zone, like a resolver in front of an authoritative server.
type fakeDNS struct {
	cached, zone string // A answer for app.example.test
	cachedTTL    uint32
	zoneNS       string
}

func (f fakeDNS) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	q := r.Question[0]
	ans := f.zone
	ttl := uint32(60)
	if r.RecursionDesired {
		ans, ttl = f.cached, f.cachedTTL
	}
	soa := &dns.SOA{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600}, Ns: "ns1.example.test.", Mbox: "h.example.test.", Minttl: 900}
	switch {
	case q.Qtype == dns.TypeNS && q.Name == "example.test.":
		ns := "ns1.example.test."
		if !r.RecursionDesired && f.zoneNS != "" {
			ns = f.zoneNS
		}
		m.Answer = append(m.Answer, &dns.NS{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 300}, Ns: ns})
	case q.Qtype == dns.TypeSOA:
		m.Answer = append(m.Answer, soa)
	case q.Qtype == dns.TypeA && q.Name == "ns1.example.test.":
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP("127.0.0.1")})
	case q.Qtype == dns.TypeA && q.Name == "app.example.test." && ans != "":
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: net.ParseIP(ans)})
	default:
		m.Ns = append(m.Ns, soa)
	}
	_ = w.WriteMsg(m)
}

func startFake(t *testing.T, f fakeDNS) *Inspector {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: f}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	addr := pc.LocalAddr().String()
	_, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	return &Inspector{Public: []string{addr}, LocalServers: []string{addr}, AuthPort: p, AllowPrivate: true, Timeout: time.Second}
}

func TestInspectAuthoritativeShowsChangeWhileCacheIsStale(t *testing.T) {
	in := startFake(t, fakeDNS{cached: "104.21.17.53", cachedTTL: 1200, zone: "72.61.8.70"})
	got := in.Inspect(context.Background(), "app.example.test")
	if got.Zone != "example.test" || !got.AuthReachable {
		t.Fatalf("zone=%q reachable=%v", got.Zone, got.AuthReachable)
	}
	if len(got.Authoritative) != 1 || got.Authoritative[0].Addresses[0] != "72.61.8.70" {
		t.Fatalf("authoritative = %+v", got.Authoritative)
	}
	if got.Public[0].Addresses[0] != "104.21.17.53" || got.Public[0].TTL != 1200 {
		t.Fatalf("public = %+v", got.Public)
	}
	if got.NegativeTTL != 900 {
		t.Errorf("negative ttl = %d, want SOA minimum 900", got.NegativeTTL)
	}
	loc := in.Local(context.Background(), "app.example.test")
	if loc.TTL != 1200 || loc.Name != LocalName {
		t.Errorf("local = %+v, want the cache's TTL", loc)
	}
}

func TestInspectReportsNSMismatchAndMissingRecord(t *testing.T) {
	in := startFake(t, fakeDNS{zoneNS: "ns9.other.test."})
	got := in.Inspect(context.Background(), "app.example.test")
	if len(got.ZoneNS) != 1 || got.ZoneNS[0] != "ns9.other.test" || got.DelegatedNS[0] != "ns1.example.test" {
		t.Fatalf("zone ns=%v delegated=%v", got.ZoneNS, got.DelegatedNS)
	}
	if got.Authoritative[0].Error != "no records" || got.Authoritative[0].TTL != 900 {
		t.Errorf("authoritative = %+v, want empty answer with negative ttl", got.Authoritative[0])
	}
}

func TestAuthoritativeServersMustBePublic(t *testing.T) {
	in := &Inspector{}
	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "169.254.169.254", "192.168.1.1", "::1"} {
		if in.allowed(net.ParseIP(ip)) {
			t.Errorf("%s must not be queryable", ip)
		}
	}
	if !in.allowed(net.ParseIP("1.1.1.1")) {
		t.Error("a public address must be queryable")
	}
}

func TestPublicResolversEnv(t *testing.T) {
	t.Setenv(EnvPublicResolvers, "")
	if got := PublicResolvers(); len(got) != 3 || got[2] != "9.9.9.9:53" {
		t.Errorf("default = %v", got)
	}
	t.Setenv(EnvPublicResolvers, "208.67.222.222, 9.9.9.9:5353")
	if got := PublicResolvers(); len(got) != 2 || got[0] != "208.67.222.222:53" || got[1] != "9.9.9.9:5353" {
		t.Errorf("custom = %v", got)
	}
	t.Setenv(EnvPublicResolvers, "off")
	if PublicResolvers() != nil {
		t.Error("off must disable public queries")
	}
}
