package ingress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var unescapedPlaceholder = regexp.MustCompile(`(^|[^\\])\{(env|http)\.`)

func TestMaintenancePage_EscapesAndDefaults(t *testing.T) {
	h := NewMaintenancePageHandler(MaintenancePage{
		Title:   `<script>alert(1)</script> {env.HOME}`,
		Message: "Back at 10:00 & {http.request.header.Cookie}",
	})
	if h.StatusCode != http.StatusServiceUnavailable || h.Headers["Retry-After"][0] != "300" {
		t.Errorf("handler = %d %v", h.StatusCode, h.Headers)
	}
	if strings.Contains(h.Body, "<script>") || unescapedPlaceholder.MatchString(h.Body) {
		t.Errorf("body is not escaped:\n%s", h.Body)
	}
	for _, want := range []string{"&lt;script&gt;", `\{env.HOME\}`, "Back at 10:00 &amp;", "prefers-color-scheme:dark"} {
		if !strings.Contains(h.Body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	d := NewMaintenanceResponseHandler()
	if !strings.Contains(d.Body, DefaultMaintenanceTitle) || !strings.Contains(d.Body, "maintenance") {
		t.Errorf("default body = %s", d.Body)
	}
	long := NewMaintenancePageHandler(MaintenancePage{Title: strings.Repeat("é", 500), RetryAfterSeconds: 60})
	if strings.Count(long.Body, "é") != 2*MaxMaintenanceTitle || long.Headers["Retry-After"][0] != "60" {
		t.Errorf("title not capped at %d characters or retry wrong", MaxMaintenanceTitle)
	}
}

func TestMaintenanceGate_NoBypassServesPageOnly(t *testing.T) {
	g := &MaintenanceGate{}
	got := g.wrap([]any{NewReverseProxyHandler("127.0.0.1:1")})
	if len(got) != 1 {
		t.Fatalf("wrap = %+v", got)
	}
	sub, ok := got[0].(SubrouteHandler)
	if !ok || len(sub.Routes) != 2 {
		t.Fatalf("wrap without bypass = %T, want asset and page routes only", got[0])
	}
	if _, ok := sub.Routes[1].Handle[0].(StaticResponseHandler); !ok {
		t.Errorf("fallthrough = %T, want the page", sub.Routes[1].Handle[0])
	}
}

func TestAssetUnavailableHandler(t *testing.T) {
	h := NewAssetUnavailableHandler(0)
	if h.StatusCode != 503 || h.Body != "" || h.Headers["Cache-Control"][0] != "no-store" ||
		h.Headers["Retry-After"][0] != "300" || !strings.HasPrefix(h.Headers["Content-Type"][0], "text/plain") {
		t.Errorf("asset response = %+v", h)
	}
}

// TestMaintenance_Live_PageBypassAndRetryAfter drives the real ingress: the
// custom page, Retry-After, a client IP bypass and a health path bypass.
func TestMaintenance_Live_PageBypassAndRetryAfter(t *testing.T) {
	skipUnderLoad(t)
	var paths []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Host+r.URL.Path)
		_, _ = w.Write([]byte("real app"))
	}))
	defer backend.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := os.MkdirTemp("", "maintenance-page-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(storage) })
	dial := backend.Listener.Addr().String()
	page := MaintenancePage{Title: "Upgrading <db>", Message: "Back soon {env.HOME}", RetryAfterSeconds: 120}
	httpPort := freePort(t)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "maint", ListenAddr: fmt.Sprintf("127.0.0.1:%d", freePort(t)), HTTPPort: httpPort,
		PlainHTTP: true, TLS: true, StorageDir: storage,
		Routes: []ProxyRoute{
			{Hosts: []string{"ops.test"}, BackendDial: dial, Maintenance: &MaintenanceGate{Page: page, BypassCIDRs: []string{"127.0.0.0/8"}}},
			{
				Hosts: []string{"probe.test"}, BackendDial: dial,
				BasicAuth:   &BasicAuthAccount{Username: "ops", Password: string(hash)},
				Maintenance: &MaintenanceGate{Page: page, BypassCIDRs: []string{"10.0.0.0/8"}, BypassPaths: []string{"/healthz"}},
			},
		},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.test"}, Page: page}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	base := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	for _, host := range []string{"down.test", "probe.test"} {
		resp, body := doReq(t, "GET", base, host, "", nil)
		if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") != "120" {
			t.Errorf("%s: status=%d retry=%q", host, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
		if !strings.Contains(body, "Upgrading &lt;db&gt;") || !strings.Contains(body, "{env.HOME}") || !strings.Contains(body, "{color-scheme:light dark") {
			t.Errorf("%s page body = %s", host, body)
		}
	}
	if resp, body := doReq(t, "GET", base, "ops.test", "", nil); resp.StatusCode != http.StatusOK || body != "real app" {
		t.Errorf("bypassed client: status=%d body=%q", resp.StatusCode, body)
	}
	if resp, _ := doReq(t, "GET", base, "probe.test", "", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("non-bypassed path got %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/healthz", nil)
	req.Host = "probe.test"
	hr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = hr.Body.Close()
	if hr.StatusCode != http.StatusUnauthorized {
		t.Errorf("health path bypasses maintenance but still keeps basic auth: got %d, want 401", hr.StatusCode)
	}
	if len(paths) != 1 || paths[0] != "ops.test/" {
		t.Errorf("backend saw %v, want only the bypassed client", paths)
	}
}

// TestMaintenance_Live_AssetsNeverGetHTML pins the regression where a
// document passed the gate but its assets received the HTML page.
func TestMaintenance_Live_AssetsNeverGetHTML(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("real app"))
	}))
	defer backend.Close()
	storage, err := os.MkdirTemp("", "maintenance-assets-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(storage) })
	dial := backend.Listener.Addr().String()
	httpPort := freePort(t)
	page := MaintenancePage{RetryAfterSeconds: 90}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "maintassets", ListenAddr: fmt.Sprintf("127.0.0.1:%d", freePort(t)), HTTPPort: httpPort,
		PlainHTTP: true, TLS: true, StorageDir: storage,
		Routes: []ProxyRoute{
			{Hosts: []string{"ops.test"}, BackendDial: dial, Maintenance: &MaintenanceGate{Page: page, BypassCIDRs: []string{"127.0.0.0/8"}}},
			{Hosts: []string{"gate.test"}, BackendDial: dial, Maintenance: &MaintenanceGate{Page: page, BypassCIDRs: []string{"10.0.0.0/8"}, BypassPaths: []string{"/healthz"}}},
			{Hosts: []string{"home.test"}, BackendDial: dial, Maintenance: &MaintenanceGate{Page: page, BypassPaths: []string{"/"}}},
		},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.test"}, Page: page}},
		HoldRoutes:        []HoldRoute{{Hosts: []string{"hold.test"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	base := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	get := func(method, host, path string, hdr map[string]string) (*http.Response, string) {
		req, _ := http.NewRequest(method, base+path, nil)
		req.Host = host
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	cases := []struct {
		name, method, host, path string
		hdr                      map[string]string
		status                   int
		html                     bool
	}{
		{"document", "GET", "down.test", "/", nil, 503, true},
		{"document with query", "GET", "down.test", "/a?b=1", nil, 503, true},
		{"next chunk", "GET", "down.test", "/_next/static/chunks/app.js", nil, 503, false},
		{"css", "GET", "down.test", "/x/site.CSS?v=2", nil, 503, false},
		{"font", "GET", "gate.test", "/f/a.woff2", nil, 503, false},
		{"image under prefix", "GET", "down.test", "/docs/img/a.png", nil, 503, false},
		{"favicon", "GET", "down.test", "/favicon.ico", nil, 503, false},
		{"asset HEAD", "HEAD", "down.test", "/a.js", nil, 503, false},
		{"asset range", "GET", "down.test", "/a.mp4", map[string]string{"Range": "bytes=0-9"}, 503, false},
		{"websocket upgrade", "GET", "down.test", "/ws", map[string]string{"Connection": "Upgrade", "Upgrade": "websocket"}, 503, true},
		{"hold asset", "GET", "hold.test", "/_next/static/a.js", nil, 503, false},
		{"hold document", "GET", "hold.test", "/", nil, 503, true},
		{"bypass client document", "GET", "ops.test", "/", nil, 200, false},
		{"bypass client asset", "GET", "ops.test", "/_next/static/a.js", nil, 200, false},
		{"health path bypass", "GET", "gate.test", "/healthz", nil, 200, false},
		{"homepage bypass leaves assets gated", "GET", "home.test", "/_next/static/a.js", nil, 503, false},
	}
	for _, tc := range cases {
		resp, body := get(tc.method, tc.host, tc.path, tc.hdr)
		if resp.StatusCode != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.status)
		}
		if tc.status != 503 {
			continue
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("%s: Cache-Control = %q", tc.name, cc)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Errorf("%s: no Retry-After", tc.name)
		}
		isHTML := strings.Contains(resp.Header.Get("Content-Type"), "text/html")
		if isHTML != tc.html || (!tc.html && body != "") {
			t.Errorf("%s: html=%v body=%q, want html=%v and empty asset body", tc.name, isHTML, body, tc.html)
		}
	}
}
