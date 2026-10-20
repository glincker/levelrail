package ingress

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func newStaticSiteForTest(t *testing.T, in staticroutes.Input) *staticSiteModule {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"index.html": "HOME", "404.html": "GONE", "about.html": "ABOUT", "docs/index.html": "DOCS", "app.js": "console.log(1)",
	} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := staticroutes.Compile(in, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(NewStaticSiteHandler(dir, rs))
	if err != nil {
		t.Fatal(err)
	}
	var m staticSiteModule
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Provision(caddy.Context{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Cleanup() })
	return &m
}

func TestStaticSiteHandlerServes(t *testing.T) {
	m := newStaticSiteForTest(t, staticroutes.Input{
		Redirects: "/old /about.html 301\n/team /about.html 200\n/blocked /about.html 404\n/go/* /:splat\n",
		Headers:   "/*\n  X-Frame-Options: DENY\n/app.js\n  Cache-Control: max-age=60\n",
		Options:   staticroutes.Options{SPA: true},
	})
	tests := []struct {
		name, method, target string
		status               int
		body, location       string
	}{
		{"home", "GET", "/", 200, "HOME", ""},
		{"file", "GET", "/app.js", 200, "console.log(1)", ""},
		{"redirect", "GET", "/old?x=1", 301, "", "/about.html?x=1"},
		{"rewrite", "GET", "/team", 200, "ABOUT", ""},
		{"status page", "GET", "/blocked", 404, "ABOUT", ""},
		{"spa fallback", "GET", "/app/deep/link", 200, "HOME", ""},
		{"missing asset 404 page", "GET", "/nope.css", 404, "GONE", ""},
		{"dir redirect", "GET", "/docs", 308, "", "/docs/"},
		{"open redirect attempt", "GET", "/go//evil.example.com", 301, "", "/evil.example.com"},
		{"post refused", "POST", "/", 405, "", ""},
		{"head has no body", "HEAD", "/app.js", 200, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if err := m.ServeHTTP(rec, req, nil); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status || (tc.body != "" && rec.Body.String() != tc.body) || rec.Header().Get("Location") != tc.location {
				t.Fatalf("status=%d body=%q location=%q", rec.Code, rec.Body.String(), rec.Header().Get("Location"))
			}
			if tc.method == "GET" && tc.status != 405 && rec.Header().Get("X-Frame-Options") != "DENY" {
				t.Fatal("matching header block must apply to every response")
			}
		})
	}
	rec := httptest.NewRecorder()
	_ = m.ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil), nil)
	if rec.Header().Get("Cache-Control") != "max-age=60" || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestStaticSiteHandlerRejectsSymlinkEscape(t *testing.T) {
	m := newStaticSiteForTest(t, staticroutes.Input{})
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(m.Root, "leak.txt")); err != nil {
		t.Skip("symlinks unavailable")
	}
	rec := httptest.NewRecorder()
	_ = m.ServeHTTP(rec, httptest.NewRequest("GET", "/leak.txt", nil), nil)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SECRET") {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestStaticRouteUsesRulesHandlerOnlyWhenPresent(t *testing.T) {
	rs, _ := staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
	for _, tc := range []struct {
		rules *staticroutes.Ruleset
		want  string
	}{{nil, `"handler":"file_server"`}, {rs, `"handler":"levelrail_static_site"`}} {
		cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv", ListenAddr: ":80", StaticRoutes: []StaticRoute{{Hosts: []string{"a.example.com"}, RootDir: "/srv/a", Rules: tc.rules}}})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(cfg)
		if !strings.Contains(string(raw), tc.want) {
			t.Fatalf("config %s lacks %s", raw, tc.want)
		}
	}
}
