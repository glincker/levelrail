package domaindoctor

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const probeDelay = 120 * time.Millisecond

type laggyDNS struct{ fakeDNS }

func (s laggyDNS) Answers(ctx context.Context, h string) []ResolverAnswer {
	time.Sleep(probeDelay)
	return s.fakeDNS.Answers(ctx, h)
}

func (s laggyDNS) CAA(ctx context.Context, h string) ([]CAARecord, error) {
	time.Sleep(probeDelay)
	return s.fakeDNS.CAA(ctx, h)
}

type laggyTLS struct{ fakeTLS }

func (s laggyTLS) Handshake(ctx context.Context, a, sni string) (PeerCert, error) {
	time.Sleep(probeDelay)
	return s.fakeTLS.Handshake(ctx, a, sni)
}

type laggyHTTP struct{ fakeHTTP }

func (s laggyHTTP) Get(ctx context.Context, a, sc, h, p string) (HTTPResult, error) {
	time.Sleep(probeDelay)
	return s.fakeHTTP.Get(ctx, a, sc, h, p)
}

func TestRunLatencyWithSlowProbes(t *testing.T) {
	tc, hc := new(int), new(int)
	dialed := new([]string)
	probes := Probes{
		DNS:  laggyDNS{fakeDNS{answers: here()}},
		TLS:  laggyTLS{fakeTLS{cert: goodCert(), calls: tc}},
		HTTP: laggyHTTP{fakeHTTP{responses: redirectToHTTPS(), calls: hc, dialed: dialed}},
	}
	start := time.Now()
	rep := Run(context.Background(), baseFacts(), probes, Options{Now: func() time.Time { return testNow }})
	took := time.Since(start)
	t.Logf("run with %s per probe call: %s", probeDelay, took.Round(time.Millisecond))
	if rep.Status != StatusHealthy {
		t.Fatalf("status = %s", rep.Status)
	}
	if took > 6*probeDelay {
		t.Fatalf("run took %s, want under %s", took, 6*probeDelay)
	}
}

func startDelayedDNS(t *testing.T, delay time.Duration) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback udp: %v", err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		time.Sleep(delay)
		m := new(dns.Msg)
		m.SetReply(r)
		if r.Question[0].Qtype == dns.TypeA {
			m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP(serverIP)})
		}
		_ = w.WriteMsg(m)
	})}
	var wg sync.WaitGroup
	wg.Add(1)
	srv.NotifyStartedFunc = wg.Done
	go func() { _ = srv.ActivateAndServe() }()
	wg.Wait()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

func TestNetDNSAnswersLatency(t *testing.T) {
	a, b := startDelayedDNS(t, probeDelay), startDelayedDNS(t, probeDelay)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	got := NetDNS{Public: []string{a, b}}.Answers(ctx, "latency.invalid")
	took := time.Since(start)
	t.Logf("two public resolvers, %s each, A+AAAA: %s", probeDelay, took.Round(time.Millisecond))
	if len(got) != 3 {
		t.Fatalf("answers = %d, want 3", len(got))
	}
}
