package ingress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAuthGate_Live boots the embedded Caddy with an auth gate in front of a
// backend and proves the generated JSON loads and behaves end to end.
func TestAuthGate_Live(t *testing.T) {
	skipUnderLoad(t)
	var mu sync.Mutex
	var authSaw http.Header
	var authMethod, authURI string
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authSaw, authMethod, authURI = r.Header.Clone(), r.Method, r.URL.RequestURI()
		mu.Unlock()
		switch r.Header.Get("Cookie") {
		case "session=ok":
			w.Header().Set("X-User", "alice")
			w.Header().Set("X-Email", "alice@example.com")
			w.Header().Set("X-Leak", "never-copied")
			w.WriteHeader(http.StatusOK)
		case "session=slow":
			time.Sleep(2 * time.Second)
		default:
			w.Header().Set("Location", "https://login.example.com/?rd=x")
			w.Header().Add("Set-Cookie", "csrf=abc; Path=/")
			w.WriteHeader(http.StatusFound)
			_, _ = io.WriteString(w, "login please")
		}
	}))
	defer auth.Close()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = fmt.Fprintf(w, "user=%s email=%s name=%s leak=%s body=%s", //nolint:gosec // test upstream echoes headers
			r.Header.Get("X-User"), r.Header.Get("X-Email"), r.Header.Get("X-Name"), r.Header.Get("X-Leak"), body)
	}))
	defer app.Close()

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "gate",
		ListenAddr: addr,
		Routes: []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: app.Listener.Addr().String(), AuthGate: &AuthGate{
			URL: auth.URL, CheckPath: "/check", CopyHeaders: []string{"X-User", "X-Email", "X-Name"}, Timeout: time.Second,
		}}},
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["gate"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path, body string, hdr map[string]string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, "http://"+addr+path, strings.NewReader(body))
		req.Host = "app.test"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	resp, body := do(http.MethodPost, "/orders?id=7", "payload", map[string]string{
		"Cookie": "session=ok", "X-User": "mallory", "X-Name": "spoofed",
	})
	if resp.StatusCode != http.StatusOK || body != "user=alice email=alice@example.com name= leak= body=payload" {
		t.Errorf("allowed request = %d %q", resp.StatusCode, body)
	}
	mu.Lock()
	if authMethod != http.MethodGet || authURI != "/check" {
		t.Errorf("subrequest = %s %s, want GET /check", authMethod, authURI)
	}
	if authSaw.Get("X-Forwarded-Method") != http.MethodPost || authSaw.Get("X-Forwarded-Uri") != "/orders?id=7" || authSaw.Get("X-Forwarded-Host") != "app.test" {
		t.Errorf("forwarded headers = %v", authSaw)
	}
	if authSaw.Get("X-User") != "" || authSaw.Get("X-Name") != "" {
		t.Errorf("client copies reached the auth service: %v", authSaw)
	}
	mu.Unlock()

	resp, body = do(http.MethodGet, "/", "", map[string]string{"X-User": "mallory"})
	if resp.StatusCode != http.StatusFound || body != "login please" ||
		resp.Header.Get("Location") != "https://login.example.com/?rd=x" || resp.Header.Get("Set-Cookie") != "csrf=abc; Path=/" {
		t.Errorf("denied request = %d %q %v", resp.StatusCode, body, resp.Header)
	}

	resp, _ = do(http.MethodGet, "/", "", map[string]string{"Cookie": "session=slow"})
	if resp.StatusCode < 500 {
		t.Errorf("timed out auth = %d, want a 5xx fail closed", resp.StatusCode)
	}

	auth.Close()
	resp, body = do(http.MethodGet, "/", "", map[string]string{"Cookie": "session=ok"})
	if resp.StatusCode != http.StatusBadGateway || strings.Contains(body, "user=") {
		t.Errorf("auth service down = %d %q, want 502 and no backend response", resp.StatusCode, body)
	}
}
