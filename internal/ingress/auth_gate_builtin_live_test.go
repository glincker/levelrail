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

// TestAuthGate_LiveSubrequestHeaders proves the built-in provider's shape
// end to end: the secret and host headers reach the check endpoint with
// client copies overwritten, and spoofed identity headers never reach the app.
func TestAuthGate_LiveSubrequestHeaders(t *testing.T) {
	skipUnderLoad(t)
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(GateHeaderSecret) != "s3cret" || r.Header.Get(GateHeaderHost) != "app.test" || r.Header.Get(GateHeaderProto) != "http" ||
			r.Header.Get(GateHeaderClientIP) != "127.0.0.1" || r.URL.Path != "/api/v1/gate/check" || r.URL.Query().Get("_gate_token") != "T" {
			http.Error(w, fmt.Sprintf("bad subrequest %v %v", r.URL, r.Header), http.StatusTeapot)
			return
		}
		w.Header().Set("X-User", "alice@example.com")
		w.WriteHeader(http.StatusOK)
	}))
	defer auth.Close()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "user=%s alt=%s", r.Header.Get("X-User"), r.Header.Get("X_User")) //nolint:gosec // test upstream echoes headers
	}))
	defer app.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	gate := &AuthGate{
		URL: auth.URL + "/api/v1/gate", CheckPath: "/check", CopyHeaders: []string{"X-User", "X-Email", "X-Name"}, Timeout: time.Second,
		SetHeaders: map[string]string{
			GateHeaderSecret: "s3cret", GateHeaderHost: "{http.request.host}",
			GateHeaderProto: "{http.request.scheme}", GateHeaderClientIP: "{http.request.remote.host}",
		},
	}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "gate", ListenAddr: addr, HTTPPort: freePort(t), StorageDir: t.TempDir(),
		Routes: []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: app.Listener.Addr().String(), AuthGate: gate}},
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

	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/x?_gate_token=T", nil)
	req.Host = "app.test"
	req.Header.Set(GateHeaderSecret, "spoofed")
	req.Header.Set(GateHeaderHost, "evil.test")
	req.Header.Set("X-User", "mallory")
	req.Header.Set("X_User", "mallory")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "user=alice@example.com alt=" {
		t.Errorf("gated request = %d %q", resp.StatusCode, body)
	}
}

func TestAuthGate_SetHeadersRejectsReserved(t *testing.T) {
	for _, name := range []string{"Host", "Cookie", "X-Forwarded-Uri", "bad header"} {
		g := AuthGate{URL: "http://127.0.0.1:1", SetHeaders: map[string]string{name: "x"}}
		if err := g.Validate(); err == nil {
			t.Errorf("SetHeaders %q accepted", name)
		}
	}
	g := AuthGate{URL: "http://127.0.0.1:1", SetHeaders: map[string]string{GateHeaderSecret: "a\r\nb"}}
	if err := g.Validate(); err == nil {
		t.Error("header value with CRLF accepted")
	}
}
