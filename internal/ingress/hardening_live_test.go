package ingress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func applyHardenedConfig(t *testing.T, backendDial string, h Hardening, hold []HoldRoute) string {
	t.Helper()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "hardened",
		ListenAddr: addr,
		Routes:     []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: backendDial}},
		HoldRoutes: hold,
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Hardening:  &h,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["hardened"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	return "http://" + addr
}

func doReq(t *testing.T, method, url, host string, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestHardening_Live_ClientIPAndBodyLimit(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusRequestEntityTooLarge)
			return
		}
		w.Header().Set("X-Seen-Real", r.Header.Get("X-Real-IP"))
		w.Header().Set("X-Seen-Xff", r.Header.Get("X-Forwarded-For"))
		w.Header().Set("X-Seen-Bytes", fmt.Sprint(n))
	}))
	defer backend.Close()

	h := DefaultHardening()
	h.MaxBodyBytes = 16
	base := applyHardenedConfig(t, backend.Listener.Addr().String(), h, nil)

	resp, _ := doReq(t, "GET", base, "app.test", "", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if resp.Header.Get("X-Seen-Real") != "127.0.0.1" || strings.Contains(resp.Header.Get("X-Seen-Xff"), "203.0.113.9") {
		t.Errorf("untrusted XFF must be replaced by the real peer, got real=%q xff=%q", resp.Header.Get("X-Seen-Real"), resp.Header.Get("X-Seen-Xff"))
	}
	resp, _ = doReq(t, "POST", base, "app.test", strings.Repeat("x", 64), nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body status = %d, want 413", resp.StatusCode)
	}
	resp, _ = doReq(t, "POST", base, "app.test", "small", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("small body status = %d, want 200", resp.StatusCode)
	}
}

func TestHardening_Live_TrustedProxyClientIP(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Real", r.Header.Get("X-Real-IP"))
		w.Header().Set("X-Seen-Xff", r.Header.Get("X-Forwarded-For"))
	}))
	defer backend.Close()

	h := DefaultHardening()
	h.TrustedProxies = []string{"127.0.0.0/8"}
	base := applyHardenedConfig(t, backend.Listener.Addr().String(), h, nil)
	resp, _ := doReq(t, "GET", base, "app.test", "", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if resp.Header.Get("X-Seen-Real") != "203.0.113.9" {
		t.Errorf("trusted proxy XFF must become the client ip, got %q", resp.Header.Get("X-Seen-Real"))
	}
}

func TestHardening_Live_FriendlyUnavailable(t *testing.T) {
	skipUnderLoad(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	addr := dead.Listener.Addr().String()
	dead.Close()

	h := DefaultHardening()
	h.RetryWindow = 500 * time.Millisecond
	base := applyHardenedConfig(t, addr, h, []HoldRoute{{Hosts: []string{"held.test"}}})

	start := time.Now()
	resp, body := doReq(t, "GET", base, "app.test", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") != "5" || !strings.Contains(body, "retry on its own") {
		t.Errorf("dead backend: status=%d retry-after=%q body=%q", resp.StatusCode, resp.Header.Get("Retry-After"), body)
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Errorf("request returned in %s, the retry window was not honoured", time.Since(start))
	}
	resp, _ = doReq(t, "GET", base, "held.test", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Errorf("hold route status=%d", resp.StatusCode)
	}
}

func TestDriver_Live_CustomErrorPageApplies(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.NotFoundHandler())
	defer backend.Close()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "errpages",
		ListenAddr: addr,
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Routes: []ProxyRoute{{
			Hosts: []string{"app.test"}, BackendDial: backend.Listener.Addr().String(),
			ErrorPages: []ErrorPage{{StatusCode: 404, Body: "custom missing"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["errpages"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer func() { _ = d.Stop(context.Background()) }()
	resp, body := doReq(t, "GET", "http://"+addr, "app.test", "", nil)
	if resp.StatusCode != 404 || body != "custom missing" {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestHardening_Live_DeadReplicaCostsNoRequests(t *testing.T) {
	skipUnderLoad(t)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "alive") }))
	defer live.Close()
	dead := httptest.NewServer(http.NotFoundHandler())
	deadAddr := dead.Listener.Addr().String()
	dead.Close()

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	h := DefaultHardening()
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "pool",
		ListenAddr: addr,
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Hardening:  &h,
		Routes: []ProxyRoute{{
			Hosts: []string{"app.test"},
			LB:    &LBRoute{Upstreams: []string{deadAddr, live.Listener.Addr().String()}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["pool"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer func() { _ = d.Stop(context.Background()) }()

	for i := 0; i < 40; i++ {
		resp, body := doReq(t, "GET", "http://"+addr, "app.test", "", nil)
		if resp.StatusCode != http.StatusOK || body != "alive" {
			t.Fatalf("request %d: status=%d body=%q, a dead replica must be retried onto the live one", i, resp.StatusCode, body)
		}
	}
}
