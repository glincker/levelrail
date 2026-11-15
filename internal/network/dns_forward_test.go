package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type fakeUpstream struct {
	addr string
	hits atomic.Int64
	tags atomic.Int64
}

type upHandler func(up *fakeUpstream, w dns.ResponseWriter, r *dns.Msg)

// startUpstream serves on one loopback port over both UDP and TCP; a nil tcp reuses the udp handler.
func startUpstream(t *testing.T, udp, tcp upHandler) *fakeUpstream {
	t.Helper()
	if tcp == nil {
		tcp = udp
	}
	for i := 0; i < 5; i++ {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen udp: %v", err)
		}
		ln, err := net.Listen("tcp", pc.LocalAddr().String())
		if err != nil {
			_ = pc.Close()
			continue
		}
		up := &fakeUpstream{addr: pc.LocalAddr().String()}
		wrap := func(h upHandler) dns.Handler {
			return dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) { h(up, w, r) })
		}
		ready := make(chan struct{}, 2)
		u := &dns.Server{PacketConn: pc, Handler: wrap(udp), NotifyStartedFunc: func() { ready <- struct{}{} }}
		c := &dns.Server{Listener: ln, Handler: wrap(tcp), NotifyStartedFunc: func() { ready <- struct{}{} }}
		go func() { _ = u.ActivateAndServe() }()
		go func() { _ = c.ActivateAndServe() }()
		<-ready
		<-ready
		t.Cleanup(func() { _ = u.Shutdown(); _ = c.Shutdown() })
		return up
	}
	t.Fatal("no free udp and tcp port pair")
	return nil
}

func answerA60() upHandler {
	const ttl = 60
	return func(up *fakeUpstream, w dns.ResponseWriter, r *dns.Msg) {
		up.hits.Add(1)
		if opt := r.IsEdns0(); opt != nil {
			for _, o := range opt.Option {
				if l, ok := o.(*dns.EDNS0_LOCAL); ok && l.Code == loopTagCode && string(l.Data) == "node-1" {
					up.tags.Add(1)
				}
			}
		}
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: net.ParseIP("203.0.113.7")}}
		_ = w.WriteMsg(m)
	}
}

func rcodeOnly(rcode int, soaMin uint32) upHandler {
	return func(up *fakeUpstream, w dns.ResponseWriter, r *dns.Msg) {
		up.hits.Add(1)
		m := new(dns.Msg)
		m.SetRcode(r, rcode)
		if soaMin > 0 {
			m.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: soaMin}, Ns: "ns.example.test.", Mbox: "h.example.test.", Minttl: soaMin}}
		}
		_ = w.WriteMsg(m)
	}
}

func testForwarder(ups ...string) *Forwarder {
	return NewForwarder(ForwarderConfig{
		NodeID:         "node-1",
		Upstreams:      func() []string { return ups },
		AttemptTimeout: func() time.Duration { return 150 * time.Millisecond },
		QueryTimeout:   func() time.Duration { return 2 * time.Second },
		CacheBytes:     func() int { return 1 << 20 },
		CacheMaxTTL:    func() time.Duration { return time.Hour },
		CacheNegTTL:    func() time.Duration { return 5 * time.Minute },
		RateQPS:        func() int { return 0 },
		RateBurst:      func() int { return 1 },
		Logger:         quietLogger(),
	})
}

var testSrc = netip.MustParseAddr("172.17.0.2")

func resolveA(t *testing.T, f *Forwarder, name string) (*dns.Msg, string, error) {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	return f.Resolve(context.Background(), testSrc, m, nil)
}

func TestForwarder_ForwardsAndTagsQuery(t *testing.T) {
	up := startUpstream(t, answerA60(), nil)
	f := testForwarder(up.addr)
	resp, reason, err := resolveA(t, f, "example.test")
	if err != nil || reason != ReasonForwarded || len(resp.Answer) != 1 {
		t.Fatalf("resp=%v reason=%q err=%v", resp, reason, err)
	}
	if up.tags.Load() != 1 {
		t.Fatal("upstream did not see the loop tag")
	}
}

func TestForwarder_TimeoutThenFailover(t *testing.T) {
	dead, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dead.Close() })
	live := startUpstream(t, answerA60(), nil)
	f := testForwarder(dead.LocalAddr().String(), live.addr)
	if _, reason, err := resolveA(t, f, "a.test"); err != nil || reason != ReasonForwarded {
		t.Fatalf("reason=%q err=%v", reason, err)
	}
	if got := f.ordered([]string{dead.LocalAddr().String(), live.addr}); got[0] != live.addr {
		t.Fatalf("failing upstream should sort last, got %v", got)
	}
}

func TestForwarder_AllUpstreamsFail(t *testing.T) {
	dead, _ := net.ListenPacket("udp", "127.0.0.1:0")
	t.Cleanup(func() { _ = dead.Close() })
	f := testForwarder(dead.LocalAddr().String())
	if _, reason, err := resolveA(t, f, "a.test"); err == nil || reason != ReasonUpstreamFailed {
		t.Fatalf("reason=%q err=%v", reason, err)
	}
}

func TestForwarder_TruncatedRetriesOverTCP(t *testing.T) {
	var tcpHits atomic.Int64
	up := startUpstream(t,
		func(_ *fakeUpstream, w dns.ResponseWriter, r *dns.Msg) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Truncated = true
			_ = w.WriteMsg(m)
		},
		func(up *fakeUpstream, w dns.ResponseWriter, r *dns.Msg) {
			tcpHits.Add(1)
			answerA60()(up, w, r)
		})
	f := testForwarder(up.addr)
	resp, _, err := resolveA(t, f, "big.test")
	if err != nil || resp.Truncated || len(resp.Answer) != 1 || tcpHits.Load() != 1 {
		t.Fatalf("resp=%v err=%v tcp=%d", resp, err, tcpHits.Load())
	}
}

func TestForwarder_DropsOwnLoopTag(t *testing.T) {
	f := testForwarder("127.0.0.1:1")
	m := new(dns.Msg)
	m.SetQuestion("x.test.", dns.TypeA)
	if f.IsLoop(m) {
		t.Fatal("untagged query flagged as a loop")
	}
	if !f.IsLoop(f.outbound(m)) || f.LoopDrops() != 1 {
		t.Fatalf("tagged query not dropped, drops=%d", f.LoopDrops())
	}
}

func TestForwarder_CacheHitAndTTLDecrement(t *testing.T) {
	up := startUpstream(t, answerA60(), nil)
	f := testForwarder(up.addr)
	now := time.Now()
	f.cache.now = func() time.Time { return now }
	if _, reason, _ := resolveA(t, f, "c.test"); reason != ReasonForwarded {
		t.Fatalf("first reason %q", reason)
	}
	now = now.Add(10 * time.Second)
	resp, reason, err := resolveA(t, f, "C.test")
	if err != nil || reason != ReasonCached || up.hits.Load() != 1 {
		t.Fatalf("reason=%q hits=%d err=%v", reason, up.hits.Load(), err)
	}
	if ttl := resp.Answer[0].Header().Ttl; ttl != 50 {
		t.Fatalf("ttl %d, want 50", ttl)
	}
	now = now.Add(time.Minute)
	if _, reason, _ := resolveA(t, f, "c.test"); reason != ReasonForwarded {
		t.Fatalf("expired entry served, reason %q", reason)
	}
}

func TestForwarder_NegativeCacheNeedsSOA(t *testing.T) {
	withSOA := startUpstream(t, rcodeOnly(dns.RcodeNameError, 30), nil)
	f := testForwarder(withSOA.addr)
	_, _, _ = resolveA(t, f, "gone.test")
	if _, reason, _ := resolveA(t, f, "gone.test"); reason != ReasonCached || withSOA.hits.Load() != 1 {
		t.Fatalf("SOA negative answer not cached, reason=%q hits=%d", reason, withSOA.hits.Load())
	}
	noSOA := startUpstream(t, rcodeOnly(dns.RcodeNameError, 0), nil)
	g := testForwarder(noSOA.addr)
	_, _, _ = resolveA(t, g, "gone.test")
	_, _, _ = resolveA(t, g, "gone.test")
	if noSOA.hits.Load() != 2 {
		t.Fatalf("negative answer without SOA was cached, hits=%d", noSOA.hits.Load())
	}
}

func TestForwarder_ServfailIsAnAnswerAndNotCached(t *testing.T) {
	up := startUpstream(t, rcodeOnly(dns.RcodeServerFailure, 30), nil)
	f := testForwarder(up.addr)
	for i := 0; i < 2; i++ {
		resp, _, err := resolveA(t, f, "broken.test")
		if err != nil || resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("resp=%v err=%v", resp, err)
		}
	}
	if up.hits.Load() != 2 {
		t.Fatalf("SERVFAIL cached, hits=%d", up.hits.Load())
	}
	if f.state(up.addr).fails.Load() != 0 {
		t.Fatal("SERVFAIL counted against upstream health")
	}
}

func TestForwarder_RateLimitRefuses(t *testing.T) {
	up := startUpstream(t, answerA60(), nil)
	f := testForwarder(up.addr)
	f.cfg.RateQPS = func() int { return 1 }
	f.cfg.RateBurst = func() int { return 2 }
	now := time.Now()
	f.now = func() time.Time { return now }
	for _, n := range []string{"a.test", "b.test"} {
		if _, _, err := resolveA(t, f, n); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	if _, reason, err := resolveA(t, f, "c.test"); !errors.Is(err, ErrForwardRateLimited) || reason != ReasonRefused {
		t.Fatalf("reason=%q err=%v", reason, err)
	}
	now = now.Add(2 * time.Second)
	if _, _, err := resolveA(t, f, "c.test"); err != nil {
		t.Fatalf("after refill: %v", err)
	}
}

func TestForwarder_HostUpstreamSelection(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	sysd := write("sysd", "nameserver 127.0.0.53\nnameserver 10.0.0.2\nnameserver 172.17.0.1\n")
	etc := write("etc", "nameserver 127.0.0.53\n")
	self := []netip.Addr{netip.MustParseAddr("172.17.0.1")}
	f := NewForwarder(ForwarderConfig{NodeID: "n", ResolvFiles: []string{sysd, etc}, Self: func() []netip.Addr { return self }})
	if got := f.Upstreams(); len(got) != 1 || got[0] != "10.0.0.2:53" {
		t.Fatalf("upstreams %v", got)
	}
	if f.FallbackAddr() != "10.0.0.2" {
		t.Fatalf("fallback %q", f.FallbackAddr())
	}
	only := NewForwarder(ForwarderConfig{NodeID: "n", ResolvFiles: []string{filepath.Join(dir, "missing"), etc}})
	if got := only.Upstreams(); len(got) != 1 || got[0] != "127.0.0.53:53" {
		t.Fatalf("stub should stay when it is the only way out: %v", got)
	}
	if only.FallbackAddr() != "" {
		t.Fatalf("a loopback stub is no container fallback, got %q", only.FallbackAddr())
	}
}

func TestDNSServer_ForwardsOutsideZone(t *testing.T) {
	up := startUpstream(t, answerA60(), nil)
	srv := startTestDNS(t, NewResolver(Zone("acme")))
	if resp := query(t, srv, "example.test", dns.TypeA); resp.Rcode != dns.RcodeRefused {
		t.Fatalf("without a forwarder want REFUSED, got %s", dns.RcodeToString[resp.Rcode])
	}
	srv.SetForwarder(testForwarder(up.addr))
	resp := query(t, srv, "example.test", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 || !resp.RecursionAvailable {
		t.Fatalf("forwarded answer wrong: %v", resp)
	}
}

func TestForwarder_GuardRunsPerSourceOnCacheHits(t *testing.T) {
	up := startUpstream(t, answerA60(), nil)
	f := testForwarder(up.addr)
	guard := func(src netip.Addr, _, resp *dns.Msg) (*dns.Msg, string) {
		if src != testSrc {
			return nil, ""
		}
		g := resp.Copy()
		g.Rcode = dns.RcodeNameError
		g.Answer = nil
		return g, "filtered"
	}
	other := netip.MustParseAddr("172.17.0.9")
	m := new(dns.Msg)
	m.SetQuestion("g.test.", dns.TypeA)
	first, _, err := f.Resolve(context.Background(), other, m, guard)
	if err != nil || len(first.Answer) == 0 {
		t.Fatalf("unfiltered source: %v %v", first, err)
	}
	hit, reason, err := f.Resolve(context.Background(), testSrc, m, guard)
	if err != nil || reason != ReasonCached || hit.Rcode != dns.RcodeNameError || len(hit.Answer) != 0 {
		t.Fatalf("filtered source got the shared answer: reason=%q rcode=%d err=%v", reason, hit.Rcode, err)
	}
	again, _, _ := f.Resolve(context.Background(), other, m, guard)
	if len(again.Answer) == 0 {
		t.Fatal("a filtered answer leaked into the cache")
	}
}
