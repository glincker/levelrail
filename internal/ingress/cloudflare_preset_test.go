package ingress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestWithCloudflarePresetTrustsOnlyListedPeers(t *testing.T) {
	h := DefaultHardening().WithCloudflarePreset([]string{"173.245.48.0/20"})
	if !slices.Equal(h.TrustedProxies, []string{"173.245.48.0/20"}) {
		t.Fatalf("trusted = %v", h.TrustedProxies)
	}
	if !slices.Equal(h.ClientIPHeaders, []string{HeaderCFConnectingIP}) {
		t.Fatalf("headers = %v", h.ClientIPHeaders)
	}
	var s Server
	h.applyServer(&s)
	if s.TrustedProxies == nil || s.TrustedProxiesStrict != 1 {
		t.Fatalf("server must trust strictly: %+v", s)
	}
}

func TestWithCloudflarePresetKeepsOperatorProxies(t *testing.T) {
	base := DefaultHardening()
	base.TrustedProxies = []string{"10.0.0.0/8"}
	h := base.WithCloudflarePreset([]string{"173.245.48.0/20"})
	if !slices.Contains(h.TrustedProxies, "10.0.0.0/8") || !slices.Contains(h.TrustedProxies, "173.245.48.0/20") {
		t.Fatalf("trusted = %v", h.TrustedProxies)
	}
	if !slices.Equal(h.ClientIPHeaders, []string{HeaderCFConnectingIP, "X-Forwarded-For"}) {
		t.Fatalf("headers = %v", h.ClientIPHeaders)
	}
	if len(base.TrustedProxies) != 1 {
		t.Fatal("the input hardening must not be mutated")
	}
}

// clientIPSeenByBackend sends one request through a live Caddy and returns the
// X-Real-IP the backend received.
func clientIPSeenByBackend(t *testing.T, h Hardening, spoof string) string {
	t.Helper()
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, r.Header.Get("X-Real-IP")) //nolint:gosec // test echo, never rendered as HTML
	}))
	defer backend.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg, err := BuildProxyConfig(ProxyOptions{ServerName: "cfip", ListenAddr: addr, BackendDial: backend.Listener.Addr().String(), StorageDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range cfg.Apps.HTTP.Servers {
		h.applyServer(s)
		for _, r := range s.Routes {
			for i, hd := range r.Handle {
				if rp, ok := hd.(ReverseProxyHandler); ok {
					h.applyProxy(&rp)
					r.Handle[i] = rp
				}
			}
		}
	}
	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Stop(ctx) }()
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", http.NoBody)
	req.Header.Set(HeaderCFConnectingIP, spoof)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestCloudflarePresetLiveCaddyHonoursHeaderOnlyFromRanges(t *testing.T) {
	inside := DefaultHardening().WithCloudflarePreset([]string{"127.0.0.0/8"})
	if got := clientIPSeenByBackend(t, inside, "198.51.100.7"); got != "198.51.100.7" {
		t.Fatalf("peer inside ranges: backend saw %q", got)
	}
	outside := DefaultHardening().WithCloudflarePreset([]string{"173.245.48.0/20"})
	if got := clientIPSeenByBackend(t, outside, "198.51.100.7"); got != "127.0.0.1" {
		t.Fatalf("peer outside ranges: backend saw %q", got)
	}
}
