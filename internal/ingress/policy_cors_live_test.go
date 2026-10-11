package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestTrafficPolicy_Live_CORSRegexForceHTTPS(t *testing.T) {
	skipUnderLoad(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Upgrade", r.Header.Get("Upgrade"))
		w.Header().Set("X-Seen-Path", r.URL.Path)
		_, _ = fmt.Fprintf(w, "app %s", r.URL.Path) //nolint:gosec // test upstream echoes the path
	}))
	defer app.Close()
	noWS := false
	policy := trafficpolicy.Policy{
		Redirects: &trafficpolicy.Redirects{ForceHTTPS: true},
		Headers: &trafficpolicy.Headers{
			CORS:     &trafficpolicy.CORSPreset{Origins: []string{"https://ui.example.com"}, Methods: []string{"GET", "PUT"}, Credentials: true, MaxAgeSeconds: 600, Preflight: true},
			Security: &trafficpolicy.SecurityPreset{HSTS: true, HSTSMaxAge: 600},
		},
		Forwarders: &trafficpolicy.Forwarders{Rules: []trafficpolicy.Forwarder{
			{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchRegex, Path: `^/v[0-9]+/`, Methods: []string{"GET"}}, Action: trafficpolicy.ActionApp, App: "self", RewritePrefix: "/versioned", WebSocket: &noWS},
		}},
	}
	rp := &RoutePolicy{
		Policy: policy, AppDials: map[string]string{"self": app.Listener.Addr().String()},
		TLSTerminatedUpstream: true, HTTPSPort: 8443, Limits: trafficpolicy.DefaultLimits(),
	}
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	h := DefaultHardening()
	h.TrustedProxies = []string{"127.0.0.1/32"}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "policy2", ListenAddr: addr, HTTPPort: freePort(t), StorageDir: t.TempDir(), Hardening: &h,
		Routes: []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: app.Listener.Addr().String(), Policy: rp}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["policy2"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path string, hdr map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, "http://"+addr+path, nil)
		req.Host = "app.test"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}

	if resp := do("GET", "/a?x=1", map[string]string{"X-Forwarded-Proto": "http"}); resp.StatusCode != 308 || resp.Header.Get("Location") != "https://app.test:8443/a?x=1" {
		t.Errorf("force https = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp := do("GET", "/a", map[string]string{"X-Forwarded-Proto": "https"})
	if resp.StatusCode != 200 || resp.Header.Get("Strict-Transport-Security") != "max-age=600" {
		t.Errorf("https request = %d hsts %q", resp.StatusCode, resp.Header.Get("Strict-Transport-Security"))
	}
	pre := do("OPTIONS", "/a", map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://ui.example.com", "Access-Control-Request-Method": "PUT"})
	if pre.StatusCode != 204 || pre.Header.Get("Access-Control-Allow-Origin") != "https://ui.example.com" || pre.Header.Get("Access-Control-Allow-Methods") != "GET, PUT" || pre.Header.Get("Access-Control-Max-Age") != "600" {
		t.Errorf("preflight = %d %v", pre.StatusCode, pre.Header)
	}
	if bad := do("OPTIONS", "/a", map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://evil.example", "Access-Control-Request-Method": "PUT"}); bad.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("disallowed origin got CORS headers: %v", bad.Header)
	}
	if cors := do("GET", "/a", map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://ui.example.com"}); cors.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("simple CORS = %v", cors.Header)
	}
	v := do("GET", "/v2/x", map[string]string{"X-Forwarded-Proto": "https", "Upgrade": "websocket"})
	if v.Header.Get("X-Seen-Upgrade") != "" || v.Header.Get("X-Seen-Path") != "/versioned/v2/x" {
		t.Errorf("regex forwarder: upgrade %q path %q", v.Header.Get("X-Seen-Upgrade"), v.Header.Get("X-Seen-Path"))
	}
	if spoof := do("GET", "/a", map[string]string{"X-Forwarded-Proto": "http", "X-Forwarded-For": "203.0.113.4"}); spoof.StatusCode != 308 {
		t.Errorf("trusted peer marking http must redirect, got %d", spoof.StatusCode)
	}
}
