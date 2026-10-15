package ingress

import (
	"context"
	"fmt"
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
	if _, ok := got[0].(StaticResponseHandler); !ok {
		t.Errorf("wrap without bypass = %T, want the page alone", got[0])
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
