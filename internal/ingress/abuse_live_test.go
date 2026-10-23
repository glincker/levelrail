package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/abuse"
)

// TestAbuseDefense_Live boots the embedded Caddy with the abuse handler and
// drives real requests through it: the generated JSON loads, the client
// address comes from Caddy's trusted proxy handling, and enforce answers 429.
func TestAbuseDefense_Live(t *testing.T) {
	skipUnderLoad(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "app")
	}))
	defer upstream.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	h := DefaultHardening()
	h.TrustedProxies = []string{"127.0.0.1/32"}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "abuse",
		ListenAddr: addr,
		Routes: []ProxyRoute{{
			Hosts: []string{"app.test"}, BackendDial: upstream.Listener.Addr().String(),
			Abuse: &AbusePolicy{App: "abuse-live", Policy: abuse.Policy{Mode: abuse.ModeEnforce, RPS: 1, Burst: 1, DenyUserAgents: []string{"badbot"}}},
		}},
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Hardening:  &h,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["abuse"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	get := func(forwardedFor, ua string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
		req.Host = "app.test"
		req.Header.Set("X-Forwarded-For", forwardedFor)
		req.Header.Set("User-Agent", ua)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	if r := get("198.51.100.1", "ok"); r.StatusCode != http.StatusOK {
		t.Fatalf("first request = %d", r.StatusCode)
	}
	r := get("198.51.100.1", "ok")
	if r.StatusCode != http.StatusTooManyRequests || r.Header.Get("Retry-After") == "" {
		t.Fatalf("over the limit = %d retry-after %q, want 429 with Retry-After", r.StatusCode, r.Header.Get("Retry-After"))
	}
	if r := get("198.51.100.2", "ok"); r.StatusCode != http.StatusOK {
		t.Fatalf("another client behind the trusted proxy = %d", r.StatusCode)
	}
	if r := get("198.51.100.3", "BadBot/1"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("denied agent = %d", r.StatusCode)
	}
	snap := abuse.DefaultRegistry().DrainAll()["abuse-live"]
	if snap.Blocked[abuse.ReasonRateLimit] != 1 || snap.Blocked[abuse.ReasonDenyUserAgent] != 1 || snap.Allowed != 2 {
		t.Fatalf("counters = %+v", snap)
	}
}
