package abuse

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ ns int64 }

func (f *fakeClock) now() int64              { return f.ns }
func (f *fakeClock) advance(d time.Duration) { f.ns += int64(d) }

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }

func newTestEngine(t *testing.T, p Policy, clk *fakeClock) (*Engine, *State) {
	t.Helper()
	l := DefaultLimits()
	st := &State{Limiter: NewLimiter(l.MaxTrackedClients, 4, l.IdleEvict, clk.now), Counters: &Counters{}}
	e, err := NewEngine(p, st, l)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return e, st
}

func TestLimiter_BurstRefillAndIsolation(t *testing.T) {
	tests := []struct {
		name  string
		steps func(t *testing.T, l *Limiter, clk *fakeClock)
	}{
		{"burst then refuse", func(t *testing.T, l *Limiter, _ *fakeClock) {
			for i := 0; i < 3; i++ {
				if ok, _ := l.Allow(ip("1.1.1.1"), 1, 3); !ok {
					t.Fatalf("request %d refused inside the burst", i)
				}
			}
			ok, wait := l.Allow(ip("1.1.1.1"), 1, 3)
			if ok || wait != time.Second {
				t.Fatalf("4th request = %v, wait %v; want refused with 1s", ok, wait)
			}
		}},
		{"refill after waiting", func(t *testing.T, l *Limiter, clk *fakeClock) {
			l.Allow(ip("1.1.1.1"), 2, 1)
			if ok, _ := l.Allow(ip("1.1.1.1"), 2, 1); ok {
				t.Fatal("second immediate request allowed")
			}
			clk.advance(500 * time.Millisecond)
			if ok, _ := l.Allow(ip("1.1.1.1"), 2, 1); !ok {
				t.Fatal("request refused after one token refilled")
			}
		}},
		{"refill never exceeds burst", func(t *testing.T, l *Limiter, clk *fakeClock) {
			l.Allow(ip("1.1.1.1"), 10, 2)
			clk.advance(time.Hour)
			allowed := 0
			for i := 0; i < 5; i++ {
				if ok, _ := l.Allow(ip("1.1.1.1"), 10, 2); ok {
					allowed++
				}
			}
			if allowed != 2 {
				t.Fatalf("allowed %d after a long idle, want the burst of 2", allowed)
			}
		}},
		{"clients are isolated", func(t *testing.T, l *Limiter, _ *fakeClock) {
			l.Allow(ip("1.1.1.1"), 1, 1)
			if ok, _ := l.Allow(ip("1.1.1.1"), 1, 1); ok {
				t.Fatal("first client not limited")
			}
			if ok, _ := l.Allow(ip("2.2.2.2"), 1, 1); !ok {
				t.Fatal("second client limited by the first")
			}
			if ok, _ := l.Allow(ip("2001:db8::1"), 1, 1); !ok {
				t.Fatal("ipv6 client limited by an ipv4 one")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := &fakeClock{}
			tt.steps(t, NewLimiter(1000, 4, time.Minute, clk.now), clk)
		})
	}
}

func TestLimiter_EvictionBoundsMemory(t *testing.T) {
	clk := &fakeClock{}
	l := NewLimiter(8, 2, time.Minute, clk.now)
	for i := 0; i < 500; i++ {
		l.Allow(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8 & 0xff), byte(i & 0xff)}), 1, 1)
	}
	if got := l.Len(); got > 8 {
		t.Fatalf("tracked %d clients, cap is 8", got)
	}
	if l.Evicted() == 0 {
		t.Fatal("no eviction counted although the cap was exceeded")
	}
}

func TestLimiter_IdleClientsAreSwept(t *testing.T) {
	clk := &fakeClock{}
	l := NewLimiter(1000, 2, time.Minute, clk.now)
	for i := 0; i < 20; i++ {
		l.Allow(netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}), 1, 1)
	}
	clk.advance(2 * time.Minute)
	for i := 0; i < 20; i++ {
		l.Allow(netip.AddrFrom4([4]byte{9, 9, 9, byte(i)}), 1, 1)
	}
	if got := l.Len(); got != 20 {
		t.Fatalf("tracked %d clients after everyone went idle, want only the 20 fresh ones", got)
	}
}

func TestLimiter_ConcurrentUse(_ *testing.T) {
	l := NewLimiter(1000, 8, time.Minute, nil)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				l.Allow(netip.AddrFrom4([4]byte{10, 1, byte(g & 0xff), byte(i & 0xff)}), 1000, 10)
			}
		}()
	}
	wg.Wait()
}

func TestEngine_Modes(t *testing.T) {
	base := Policy{RPS: 1, Burst: 1, DenyCIDRs: []string{"203.0.113.0/24", "198.51.100.7"}, DenyUserAgents: []string{"BadBot"}, RequireUserAgent: true}
	tests := []struct {
		name       string
		mode       Mode
		client     string
		ua         string
		wantAction Action
		wantReason Reason
	}{
		{"off allows everything", ModeOff, "203.0.113.5", "", ActionAllow, 0},
		{"enforce blocks denied cidr", ModeEnforce, "203.0.113.5", "curl", ActionBlock, ReasonDenyCIDR},
		{"enforce blocks single ip entry", ModeEnforce, "198.51.100.7", "curl", ActionBlock, ReasonDenyCIDR},
		{"enforce blocks mapped ipv6 of denied v4", ModeEnforce, "::ffff:203.0.113.9", "curl", ActionBlock, ReasonDenyCIDR},
		{"enforce blocks missing ua", ModeEnforce, "192.0.2.1", "", ActionBlock, ReasonMissingUserAgent},
		{"enforce blocks ua substring any case", ModeEnforce, "192.0.2.1", "xx-bADbOt/1.0", ActionBlock, ReasonDenyUserAgent},
		{"enforce allows clean request", ModeEnforce, "192.0.2.1", "Mozilla/5.0", ActionAllow, 0},
		{"log_only reports deny cidr but allows", ModeLogOnly, "203.0.113.5", "curl", ActionWouldBlock, ReasonDenyCIDR},
		{"log_only reports missing ua but allows", ModeLogOnly, "192.0.2.1", "", ActionWouldBlock, ReasonMissingUserAgent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			p.Mode = tt.mode
			e, _ := newTestEngine(t, p, &fakeClock{})
			d := e.Decide(ip(tt.client).Unmap(), tt.ua)
			if d.Action != tt.wantAction || (tt.wantAction != ActionAllow && d.Reason != tt.wantReason) {
				t.Fatalf("Decide() = %+v, want action %d reason %v", d, tt.wantAction, tt.wantReason)
			}
		})
	}
}

func TestEngine_EnforceRateLimitSetsRetryAfter(t *testing.T) {
	clk := &fakeClock{}
	e, _ := newTestEngine(t, Policy{Mode: ModeEnforce, RPS: 0.5, Burst: 2}, clk)
	c := ip("192.0.2.1")
	for i := 0; i < 2; i++ {
		if d := e.Decide(c, "x"); d.Action != ActionAllow {
			t.Fatalf("request %d blocked inside the burst", i)
		}
	}
	d := e.Decide(c, "x")
	if d.Action != ActionBlock || d.Reason != ReasonRateLimit || d.RetryAfter != 2 {
		t.Fatalf("over-limit Decide() = %+v, want block rate_limit retry 2", d)
	}
	clk.advance(2 * time.Second)
	if d := e.Decide(c, "x"); d.Action != ActionAllow {
		t.Fatalf("still blocked after the wait: %+v", d)
	}
}

func TestEngine_RetryAfterCapped(t *testing.T) {
	l := DefaultLimits()
	l.MaxRetryAfter = 5 * time.Second
	clk := &fakeClock{}
	st := &State{Limiter: NewLimiter(100, 2, l.IdleEvict, clk.now), Counters: &Counters{}}
	e, err := NewEngine(Policy{Mode: ModeEnforce, RPS: 0.001, Burst: 1}, st, l)
	if err != nil {
		t.Fatal(err)
	}
	e.Decide(ip("1.2.3.4"), "x")
	if d := e.Decide(ip("1.2.3.4"), "x"); d.RetryAfter != 5 {
		t.Fatalf("RetryAfter = %d, want the cap of 5", d.RetryAfter)
	}
}

func TestEngine_LogOnlyNeverBlocksAndCounts(t *testing.T) {
	clk := &fakeClock{}
	e, st := newTestEngine(t, Policy{Mode: ModeLogOnly, RPS: 1, Burst: 1, DenyUserAgents: []string{"scrapy"}}, clk)
	c := ip("192.0.2.9")
	for i := 0; i < 10; i++ {
		if d := e.Decide(c, "ok"); d.Action == ActionBlock {
			t.Fatalf("log_only blocked request %d", i)
		}
	}
	e.Decide(c, "Scrapy/2")
	s := st.Counters.Drain()
	if s.Allowed != 1 || s.WouldBlock[ReasonRateLimit] != 9 || s.WouldBlock[ReasonDenyUserAgent] != 1 {
		t.Fatalf("counters = %+v", s)
	}
	for r, n := range s.Blocked {
		if n != 0 {
			t.Fatalf("log_only counted %d real blocks for reason %d", n, r)
		}
	}
}

func TestEngine_EnforceCountsBlocked(t *testing.T) {
	e, st := newTestEngine(t, Policy{Mode: ModeEnforce, DenyCIDRs: []string{"10.0.0.0/8"}}, &fakeClock{})
	e.Decide(ip("10.1.2.3"), "x")
	e.Decide(ip("8.8.8.8"), "x")
	s := st.Counters.Drain()
	if s.Allowed != 1 || s.Blocked[ReasonDenyCIDR] != 1 {
		t.Fatalf("counters = %+v", s)
	}
	if !st.Counters.Drain().Zero() {
		t.Fatal("Drain did not reset")
	}
}

func TestEngine_DeniedClientDoesNotSpendTokens(t *testing.T) {
	e, st := newTestEngine(t, Policy{Mode: ModeEnforce, RPS: 1, Burst: 1, DenyCIDRs: []string{"10.0.0.0/8"}}, &fakeClock{})
	for i := 0; i < 5; i++ {
		e.Decide(ip("10.1.2.3"), "x")
	}
	if st.Limiter.Len() != 0 {
		t.Fatal("a denied client reached the rate limiter")
	}
}

func TestEngine_UnknownClientSkipsAddressChecks(t *testing.T) {
	e, _ := newTestEngine(t, Policy{Mode: ModeEnforce, RPS: 1, Burst: 1, DenyCIDRs: []string{"10.0.0.0/8"}}, &fakeClock{})
	for i := 0; i < 3; i++ {
		if d := e.Decide(netip.Addr{}, "x"); d.Action != ActionAllow {
			t.Fatalf("unknown client blocked: %+v", d)
		}
	}
}

func TestPolicy_Validate(t *testing.T) {
	l := DefaultLimits()
	tests := []struct {
		name    string
		p       Policy
		wantErr bool
	}{
		{"off empty is fine", Policy{Mode: ModeOff}, false},
		{"enforce with nothing on", Policy{Mode: ModeEnforce}, true},
		{"rate only", Policy{Mode: ModeEnforce, RPS: 5, Burst: 10}, false},
		{"burst without rate", Policy{Mode: ModeEnforce, Burst: 5, RequireUserAgent: true}, true},
		{"rps above cap", Policy{Mode: ModeEnforce, RPS: l.MaxRPS + 1}, true},
		{"bad cidr", Policy{Mode: ModeEnforce, DenyCIDRs: []string{"not-an-ip"}}, true},
		{"bad mode", Policy{Mode: "block"}, true},
		{"control char in agent", Policy{Mode: ModeEnforce, DenyUserAgents: []string{"bad\nbot"}}, true},
		{"too many cidrs", Policy{Mode: ModeEnforce, DenyCIDRs: manyCIDRs(l.MaxDenyEntries + 1)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(l)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			var ve *ValidationError
			if err != nil && !errors.As(err, &ve) {
				t.Fatalf("error %T is not a *ValidationError", err)
			}
		})
	}
}

func TestPolicy_NormalizeFillsBurst(t *testing.T) {
	l := DefaultLimits()
	p := Policy{RPS: 10}.Normalize(l)
	if p.Mode != ModeOff || p.Burst != int(10*l.SuggestBurstSeconds) {
		t.Fatalf("Normalize() = %+v", p)
	}
}

func manyCIDRs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "10." + strconv.Itoa(i>>8) + "." + strconv.Itoa(i&255) + ".0/24"
	}
	return out
}

func TestResolveClientIP(t *testing.T) {
	trusted, err := ParseTrusted("10.0.0.0/8, 192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{"no proxy uses the peer", "203.0.113.9:4000", nil, "203.0.113.9"},
		{"spoofed header from an untrusted peer is ignored", "203.0.113.9:4000", []string{"1.2.3.4"}, "203.0.113.9"},
		{"trusted proxy forwards the real client", "10.0.0.5:4000", []string{"198.51.100.2"}, "198.51.100.2"},
		{"spoofed leftmost hop behind a trusted proxy is ignored", "10.0.0.5:4000", []string{"6.6.6.6, 198.51.100.2"}, "198.51.100.2"},
		{"chain of trusted proxies is skipped", "10.0.0.5:4000", []string{"198.51.100.2, 10.0.0.9, 192.168.1.1"}, "198.51.100.2"},
		{"split header lines are walked from the last", "10.0.0.5:4000", []string{"6.6.6.6", "198.51.100.2"}, "198.51.100.2"},
		{"garbage hop stops at the last good address", "10.0.0.5:4000", []string{"nonsense"}, "10.0.0.5"},
		{"ipv6 peer without port", "2001:db8::1", nil, "2001:db8::1"},
		{"mapped peer is unmapped", "[::ffff:203.0.113.9]:80", nil, "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveClientIP(tt.remote, tt.xff, trusted); got.String() != tt.want {
				t.Fatalf("ResolveClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
	if got := ResolveClientIP("10.0.0.5:1", []string{"1.1.1.1"}, nil); got.String() != "10.0.0.5" {
		t.Fatalf("with no trusted proxies the header was believed: %v", got)
	}
	if got := ResolveClientIP("garbage", nil, trusted); got.IsValid() {
		t.Fatalf("unparseable peer resolved to %v", got)
	}
}

func TestParseTrusted(t *testing.T) {
	if _, err := ParseTrusted("10.0.0.0/8 not-an-ip"); err == nil {
		t.Fatal("bad entry accepted")
	}
	got, err := ParseTrusted("private_ranges,203.0.113.1")
	if err != nil || len(got) != len(PrivateRanges())+1 {
		t.Fatalf("ParseTrusted() = %v, %v", got, err)
	}
}

func TestReplay(t *testing.T) {
	l := DefaultLimits()
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	var obs []Observation
	for i := 0; i < 30; i++ {
		obs = append(obs, Observation{Time: t0.Add(time.Duration(i) * 100 * time.Millisecond), Client: ip("192.0.2.50"), UserAgent: "Mozilla"})
	}
	obs = append(obs,
		Observation{Time: t0.Add(4 * time.Second), Client: ip("192.0.2.1"), UserAgent: "Mozilla"},
		Observation{Time: t0.Add(5 * time.Second), Client: ip("203.0.113.3"), UserAgent: "Mozilla"},
		Observation{Time: t0.Add(6 * time.Second), Client: ip("192.0.2.2"), UserAgent: ""},
	)
	p := Policy{Mode: ModeEnforce, RPS: 2, Burst: 4, DenyCIDRs: []string{"203.0.113.0/24"}, RequireUserAgent: true}
	res, err := Replay(p, obs, l, 5)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if res.Requests != 33 {
		t.Fatalf("Requests = %d, want 33", res.Requests)
	}
	// 30 requests over 2.9s at 2 rps with a burst of 4 admit 4 + 5, leaving 21.
	if res.WouldBlock != 21+2 {
		t.Fatalf("WouldBlock = %d, want 23 (21 rate, 1 cidr, 1 missing ua)", res.WouldBlock)
	}
	counts := map[string]int{}
	for _, r := range res.ByReason {
		counts[r.Reason] = r.Count
	}
	if counts["rate_limit"] != 21 || counts["deny_cidr"] != 1 || counts["missing_user_agent"] != 1 {
		t.Fatalf("ByReason = %v", counts)
	}
	if len(res.TopClients) == 0 || res.TopClients[0].Client != "192.0.2.50" || res.TopClients[0].TopReason != "rate_limit" {
		t.Fatalf("TopClients = %+v", res.TopClients)
	}
	if !res.From.Equal(t0) || res.Percent <= 0 {
		t.Fatalf("From/Percent = %v / %v", res.From, res.Percent)
	}
}

func TestReplay_HasNoSideEffectsAndHonoursDraftMode(t *testing.T) {
	l := DefaultLimits()
	obs := []Observation{{Time: time.Now(), Client: ip("203.0.113.1"), UserAgent: "x"}}
	for _, mode := range []Mode{ModeOff, ModeLogOnly, ModeEnforce} {
		res, err := Replay(Policy{Mode: mode, DenyCIDRs: []string{"203.0.113.0/24"}}, obs, l, 3)
		if err != nil || res.WouldBlock != 1 {
			t.Fatalf("mode %s: res = %+v, err = %v", mode, res, err)
		}
	}
	reg := NewRegistry(nil)
	if _, err := Replay(Policy{Mode: ModeEnforce, RPS: 1}, obs, l, 3); err != nil {
		t.Fatal(err)
	}
	if len(reg.DrainAll()) != 0 {
		t.Fatal("replay touched a live registry")
	}
}

func TestReplay_InvalidPolicyAndEmptyInput(t *testing.T) {
	l := DefaultLimits()
	if _, err := Replay(Policy{RPS: -1}, nil, l, 3); err == nil {
		t.Fatal("invalid draft accepted")
	}
	res, err := Replay(Policy{Mode: ModeEnforce, RequireUserAgent: true}, nil, l, 3)
	if err != nil || res.Requests != 0 || res.Percent != 0 {
		t.Fatalf("empty replay = %+v, %v", res, err)
	}
}

func TestSuggest(t *testing.T) {
	l := DefaultLimits()
	if s := Suggest(0, l); s.FromTraffic || s.RPS != l.DefaultRPS || s.Burst != l.DefaultBurst {
		t.Fatalf("no traffic: %+v", s)
	}
	if s := Suggest(10, l); !s.FromTraffic || s.RPS != l.DefaultRPS {
		t.Fatalf("quiet app should keep the default floor: %+v", s)
	}
	if s := Suggest(400, l); s.RPS != 100 || s.Burst != 200 {
		t.Fatalf("busy app: %+v", s)
	}
	if s := Suggest(1e9, l); s.RPS != l.MaxRPS {
		t.Fatalf("suggestion above the cap: %+v", s)
	}
}

func TestRegistry_DrainAndFailOpen(t *testing.T) {
	reg := NewRegistry(nil)
	l := DefaultLimits()
	reg.NoteFailOpen("web", l)
	reg.NoteFailOpen("web", l)
	got := reg.DrainAll()
	if got["web"].FailOpen != 2 {
		t.Fatalf("DrainAll() = %+v", got)
	}
	if len(reg.DrainAll()) != 0 {
		t.Fatal("second drain not empty")
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvMaxRPS: "50", EnvShards: "bad", EnvIdleEvict: "30s", EnvDefaultBurst: "-3"}
	l := LimitsFromEnv(func(k string) string { return env[k] })
	d := DefaultLimits()
	if l.MaxRPS != 50 || l.Shards != d.Shards || l.IdleEvict != 30*time.Second || l.DefaultBurst != d.DefaultBurst {
		t.Fatalf("LimitsFromEnv() = %+v", l)
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"Mozilla/5.0 (compatible; AhrefsBot/7.0)", "ahrefsbot", true},
		{"curl/8.1", "wget", false},
		{"abc", "abcd", false},
		{"", "a", false},
		{"anything", "", true},
	}
	for _, tt := range tests {
		if got := containsFold(tt.s, []byte(tt.sub)); got != tt.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestEngine_LongUserAgentCannotHideAToken(t *testing.T) {
	e, _ := newTestEngine(t, Policy{Mode: ModeEnforce, DenyUserAgents: []string{"badbot"}}, &fakeClock{})
	ua := strings.Repeat("a", 2000) + "BadBot"
	if d := e.Decide(ip("192.0.2.1"), ua); d.Action != ActionBlock || d.Reason != ReasonDenyUserAgent {
		t.Fatalf("padded user agent slipped through: %+v", d)
	}
}

func BenchmarkDecideAllow(b *testing.B) {
	l := DefaultLimits()
	st := &State{Limiter: NewLimiter(l.MaxTrackedClients, l.Shards, l.IdleEvict, nil), Counters: &Counters{}}
	e, err := NewEngine(Policy{
		Mode: ModeEnforce, RPS: l.MaxRPS, Burst: l.MaxBurst, RequireUserAgent: true,
		DenyCIDRs: manyCIDRs(32), DenyUserAgents: []string{"badbot", "scrapy", "python-requests"},
	}, st, l)
	if err != nil {
		b.Fatal(err)
	}
	clients := make([]netip.Addr, 1<<16)
	for i := range clients {
		clients[i] = netip.AddrFrom4([4]byte{192, 0, byte(i >> 8), byte(i)})
	}
	ua := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if e.Decide(clients[i&(1<<16-1)], ua).Action != ActionAllow {
				b.Fatal("blocked")
			}
			i++
		}
	})
}
