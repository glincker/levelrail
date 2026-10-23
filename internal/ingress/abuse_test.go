package ingress

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func newAbuseModule(t testing.TB, app string, p abuse.Policy) *abuseModule {
	t.Helper()
	m := &abuseModule{App: app, Policy: p}
	if err := m.Provision(caddy.Context{}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	return m
}

func serveAbuse(m *abuseModule, remote, ua string, hdr map[string]string) (code int, retryAfter string, reached bool) {
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	r.RemoteAddr = remote
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	} else {
		r.Header.Del("User-Agent")
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	next := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { reached = true; return nil })
	if err := m.ServeHTTP(w, r, next); err != nil {
		panic(err)
	}
	if reached {
		return http.StatusOK, "", true
	}
	return w.Code, w.Header().Get("Retry-After"), false
}

func TestAbuseModule_EnforceRateLimitReturns429WithRetryAfter(t *testing.T) {
	m := newAbuseModule(t, "enforce-rate", abuse.Policy{Mode: abuse.ModeEnforce, RPS: 1, Burst: 2})
	for i := 0; i < 2; i++ {
		if code, _, reached := serveAbuse(m, "203.0.113.7:5000", "ua", nil); code != http.StatusOK || !reached {
			t.Fatalf("request %d inside the burst: code %d", i, code)
		}
	}
	code, ra, reached := serveAbuse(m, "203.0.113.7:5000", "ua", nil)
	if code != http.StatusTooManyRequests || reached {
		t.Fatalf("over the limit: code %d reached %v, want 429 and not served", code, reached)
	}
	if n, err := strconv.Atoi(ra); err != nil || n < 1 {
		t.Fatalf("Retry-After = %q, want a positive number of seconds", ra)
	}
	if code, _, _ := serveAbuse(m, "203.0.113.8:5000", "ua", nil); code != http.StatusOK {
		t.Fatalf("a different client was limited: %d", code)
	}
}

func TestAbuseModule_EnforceDenies(t *testing.T) {
	m := newAbuseModule(t, "enforce-deny", abuse.Policy{
		Mode: abuse.ModeEnforce, DenyCIDRs: []string{"198.51.100.0/24"}, DenyUserAgents: []string{"badbot"}, RequireUserAgent: true,
	})
	tests := []struct {
		name, remote, ua string
		want             int
	}{
		{"denied cidr", "198.51.100.9:1", "ua", http.StatusForbidden},
		{"denied agent", "192.0.2.1:1", "Mozilla BadBot/1", http.StatusForbidden},
		{"missing agent", "192.0.2.1:1", "", http.StatusForbidden},
		{"clean request", "192.0.2.1:1", "Mozilla", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, ra, _ := serveAbuse(m, tt.remote, tt.ua, nil); code != tt.want || ra != "" {
				t.Fatalf("code = %d retry-after %q, want %d without Retry-After", code, ra, tt.want)
			}
		})
	}
}

func TestAbuseModule_LogOnlyNeverBlocksAndCounts(t *testing.T) {
	app := "logonly-app"
	m := newAbuseModule(t, app, abuse.Policy{Mode: abuse.ModeLogOnly, RPS: 1, Burst: 1, DenyUserAgents: []string{"scrapy"}})
	abuse.DefaultRegistry().For(app, abuse.DefaultLimits()).Counters.Drain()
	for i := 0; i < 20; i++ {
		if code, _, reached := serveAbuse(m, "203.0.113.50:1", "ua", nil); code != http.StatusOK || !reached {
			t.Fatalf("log_only turned request %d away with %d", i, code)
		}
	}
	if code, _, reached := serveAbuse(m, "203.0.113.51:1", "Scrapy", nil); code != http.StatusOK || !reached {
		t.Fatalf("log_only blocked a denied agent: %d", code)
	}
	snap := abuse.DefaultRegistry().DrainAll()[app]
	if snap.WouldBlock[abuse.ReasonRateLimit] != 19 || snap.WouldBlock[abuse.ReasonDenyUserAgent] != 1 || snap.Allowed != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	for _, n := range snap.Blocked {
		if n != 0 {
			t.Fatalf("log_only counted a real block: %+v", snap)
		}
	}
}

func TestAbuseModule_SpoofedForwardedForIgnoredWithoutTrustedProxy(t *testing.T) {
	t.Setenv(EnvTrustedProxies, "")
	m := newAbuseModule(t, "spoof-untrusted", abuse.Policy{Mode: abuse.ModeEnforce, RPS: 1, Burst: 1})
	hdr := func(i int) map[string]string {
		return map[string]string{headerForwardedFor: "9.9.9." + strconv.Itoa(i)}
	}
	if code, _, _ := serveAbuse(m, "203.0.113.7:1", "ua", hdr(1)); code != http.StatusOK {
		t.Fatalf("first request: %d", code)
	}
	if code, _, _ := serveAbuse(m, "203.0.113.7:1", "ua", hdr(2)); code != http.StatusTooManyRequests {
		t.Fatalf("rotating X-Forwarded-For dodged the limit: %d", code)
	}
}

func TestAbuseModule_ForwardedForHonouredOnlyFromTrustedProxy(t *testing.T) {
	t.Setenv(EnvTrustedProxies, "10.0.0.0/8")
	m := newAbuseModule(t, "spoof-trusted", abuse.Policy{Mode: abuse.ModeEnforce, RPS: 1, Burst: 1, DenyCIDRs: []string{"6.6.6.6"}})
	xff := func(v string) map[string]string { return map[string]string{headerForwardedFor: v} }
	if code, _, _ := serveAbuse(m, "10.1.1.1:1", "ua", xff("198.51.100.1")); code != http.StatusOK {
		t.Fatalf("real client behind the proxy: %d", code)
	}
	if code, _, _ := serveAbuse(m, "10.1.1.1:1", "ua", xff("198.51.100.1")); code != http.StatusTooManyRequests {
		t.Fatalf("same forwarded client not limited: %d", code)
	}
	if code, _, _ := serveAbuse(m, "10.1.1.1:1", "ua", xff("198.51.100.2")); code != http.StatusOK {
		t.Fatalf("a second forwarded client shares the proxy's bucket: %d", code)
	}
	// A client-supplied leftmost hop is never reached, so it cannot dodge or frame anyone.
	if code, _, _ := serveAbuse(m, "10.1.1.1:1", "ua", xff("6.6.6.6, 198.51.100.3")); code != http.StatusOK {
		t.Fatalf("leftmost spoofed hop was trusted: %d", code)
	}
}

func TestAbuseModule_ClientIPVarWins(t *testing.T) {
	m := newAbuseModule(t, "clientvar", abuse.Policy{Mode: abuse.ModeEnforce, DenyCIDRs: []string{"198.51.100.0/24"}})
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	r.RemoteAddr = "192.0.2.1:1"
	r.Header.Set("User-Agent", "ua")
	ctx := context.WithValue(r.Context(), caddyhttp.VarsCtxKey, map[string]any{caddyhttp.ClientIPVarKey: "198.51.100.77"})
	w := httptest.NewRecorder()
	_ = m.ServeHTTP(w, r.WithContext(ctx), caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { return nil }))
	if w.Code != http.StatusForbidden {
		t.Fatalf("client_ip var from Caddy was not used: %d", w.Code)
	}
}

func TestAbuseModule_FailsOpenOnPanic(t *testing.T) {
	app := "panic-app"
	m := newAbuseModule(t, app, abuse.Policy{Mode: abuse.ModeEnforce, RPS: 1})
	abuse.DefaultRegistry().For(app, abuse.DefaultLimits()).Counters.Drain()
	m.decide = func(netip.Addr, string) abuse.Decision { panic("boom") }
	if code, _, reached := serveAbuse(m, "203.0.113.7:1", "ua", nil); code != http.StatusOK || !reached {
		t.Fatalf("a panic in the policy blocked the request: %d", code)
	}
	if got := abuse.DefaultRegistry().DrainAll()[app].FailOpen; got != 1 {
		t.Fatalf("FailOpen = %d, want 1", got)
	}
}

func TestAbuseModule_FailsOpenWhenPolicyDoesNotCompile(t *testing.T) {
	app := "badpolicy-app"
	m := newAbuseModule(t, app, abuse.Policy{Mode: abuse.ModeEnforce, DenyCIDRs: []string{"nonsense"}})
	if code, _, reached := serveAbuse(m, "203.0.113.7:1", "ua", nil); code != http.StatusOK || !reached {
		t.Fatalf("an invalid policy blocked traffic: %d", code)
	}
	if got := abuse.DefaultRegistry().DrainAll()[app].FailOpen; got != 1 {
		t.Fatalf("FailOpen = %d, want 1", got)
	}
}

func TestAbusePolicy_WrapOnlyWhenActive(t *testing.T) {
	handle := []any{"proxy"}
	for _, mode := range []abuse.Mode{abuse.ModeOff, ""} {
		if got := (&AbusePolicy{App: "a", Policy: abuse.Policy{Mode: mode}}).wrap(handle); len(got) != 1 {
			t.Fatalf("mode %q added a handler", mode)
		}
	}
	if got := (*AbusePolicy)(nil).wrap(handle); len(got) != 1 {
		t.Fatal("nil policy added a handler")
	}
	got := (&AbusePolicy{App: "a", Policy: abuse.Policy{Mode: abuse.ModeLogOnly, RPS: 1}}).wrap(handle)
	if len(got) != 2 {
		t.Fatalf("active policy handlers = %d, want 2", len(got))
	}
	raw, _ := json.Marshal(got[0])
	if !strings.Contains(string(raw), `"handler":"levelrail_abuse"`) || !strings.Contains(string(raw), `"app":"a"`) {
		t.Fatalf("wire shape = %s", raw)
	}
}

func TestAbuseReasonCountsMatchTelemetry(t *testing.T) {
	if int(abuse.ReasonCount) != telemetry.AbuseReasonCount {
		t.Fatalf("reason counts differ: %d vs %d", abuse.ReasonCount, telemetry.AbuseReasonCount)
	}
	for i, name := range abuse.ReasonNames() {
		if telemetry.AbuseReasons[i] != name {
			t.Fatalf("reason %d: %q vs %q", i, telemetry.AbuseReasons[i], name)
		}
	}
}

func TestAbuseCountersReachTheDrain(t *testing.T) {
	reg := abuse.NewRegistry(nil)
	reg.NoteFailOpen("web", abuse.DefaultLimits())
	s := NewRequestStats()
	if got := s.DrainByApp(); len(got) != 0 {
		t.Fatalf("stats without a registry drained %v", got)
	}
	s.abuse = reg
	got := s.DrainByApp()
	if got["web"].Abuse.FailOpen != 1 {
		t.Fatalf("DrainByApp() = %+v", got)
	}
	names := map[string]bool{}
	for _, smp := range got["web"].Samples("service:web", time.Now()) {
		names[smp.Metric] = true
	}
	if !names[telemetry.MetricAbuseFailOpen] {
		t.Fatalf("fail-open counter missing from samples: %v", names)
	}
}

func TestAccessRecordKeepsClientOutOfJSON(t *testing.T) {
	raw, err := json.Marshal(AccessRecord{Client: netip.MustParseAddr("203.0.113.9"), UserAgent: "secret-agent", Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "203.0.113.9") || strings.Contains(string(raw), "secret-agent") {
		t.Fatalf("client details leaked into the API shape: %s", raw)
	}
}

func TestAccessLogSinceReturnsOldestFirstAndUncapped(t *testing.T) {
	l := NewAccessLog(1000)
	base := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for i := 700; i > 0; i-- {
		l.Record("k", AccessRecord{Time: base.Add(time.Duration(-i) * time.Second), Path: "/"})
	}
	got := l.Since([]string{"k"}, base.Add(-650*time.Second))
	if len(got) != 650 {
		t.Fatalf("Since returned %d records, want all 650 in the window", len(got))
	}
	if !got[0].Time.Before(got[len(got)-1].Time) {
		t.Fatal("records are not oldest first")
	}
}

type nopWriter struct{ h http.Header }

func (w nopWriter) Header() http.Header         { return w.h }
func (nopWriter) Write(p []byte) (int, error)   { return len(p), nil }
func (nopWriter) WriteHeader(int)               {}
func okNext(http.ResponseWriter, *http.Request) {}

// The allow path with no policy is just the next handler; with a policy it is
// the module in front of it. Compare the two for the per-request overhead.
func BenchmarkAbuseAllowPath(b *testing.B) {
	addrs := make([]string, 1<<16)
	for i := range addrs {
		addrs[i] = "192.0." + strconv.Itoa(i>>8&255) + "." + strconv.Itoa(i&255) + ":4000"
	}
	run := func(b *testing.B, h func(http.ResponseWriter, *http.Request) error) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
			r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
			w := nopWriter{h: http.Header{}}
			i := 0
			for pb.Next() {
				r.RemoteAddr = addrs[i&(1<<16-1)]
				_ = h(w, r)
				i++
			}
		})
	}
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { okNext(w, r); return nil })
	b.Run("no_policy", func(b *testing.B) {
		run(b, next.ServeHTTP)
	})
	limits := abuse.DefaultLimits()
	policy := abuse.Policy{
		Mode: abuse.ModeEnforce, RPS: limits.MaxRPS, Burst: limits.MaxBurst, RequireUserAgent: true,
		DenyCIDRs:      []string{"203.0.113.0/24", "198.51.100.0/24", "10.0.0.0/8", "172.16.0.0/12", "2001:db8::/32"},
		DenyUserAgents: []string{"badbot", "scrapy", "python-requests", "curl", "wget"},
	}
	b.Run("enforce_policy", func(b *testing.B) {
		m := newAbuseModule(b, "bench-enforce", policy)
		run(b, func(w http.ResponseWriter, r *http.Request) error { return m.ServeHTTP(w, r, next) })
	})
	b.Run("log_only_policy", func(b *testing.B) {
		p := policy
		p.Mode = abuse.ModeLogOnly
		m := newAbuseModule(b, "bench-logonly", p)
		run(b, func(w http.ResponseWriter, r *http.Request) error { return m.ServeHTTP(w, r, next) })
	})
}
