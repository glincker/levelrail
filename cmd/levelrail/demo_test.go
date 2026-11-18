package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/web"
)

func demoTestLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func demoTestBrand() *brand.Brand {
	return &brand.Brand{Name: "Test Platform", ShortName: "tp", BinaryName: "tp", Domain: "example.test"}
}

func TestDemoHandlerEndToEnd(t *testing.T) {
	ctx := context.Background()
	t.Setenv("APP_DATA_DIR", t.TempDir())
	db, err := openStore(ctx)
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := demo.Seed(ctx, db, demo.Options{Seed: 1, Anchor: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	tdb, err := openTelemetryStore(ctx)
	if err != nil {
		t.Fatalf("openTelemetryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = tdb.Close() })
	adb, err := openAlertingStore(ctx)
	if err != nil {
		t.Fatalf("openAlertingStore() error = %v", err)
	}
	t.Cleanup(func() { _ = adb.Close() })

	h, err := buildDemoHandler(ctx, demoTestLogger(), demoTestBrand(), db, tdb, adb, telemetry.NewLogBroadcaster())
	if err != nil {
		t.Fatalf("buildDemoHandler() error = %v", err)
	}
	do := func(method, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader("{}"))
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodGet, "/api/v1/demo", nil); !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("GET /api/v1/demo body = %s", rec.Body.String())
	}
	if rec := do(http.MethodGet, "/robots.txt", nil); rec.Body.String() != demoRobotsTxt || rec.Header().Get("X-Robots-Tag") == "" {
		t.Errorf("robots.txt = %q, header = %q", rec.Body.String(), rec.Header().Get("X-Robots-Tag"))
	}
	// Preview bots are allowed, every other crawler stays blocked and noindex.
	for _, want := range []string{"User-agent: Redditbot\n", "Allow: /\n", "User-agent: *\nDisallow: /\n"} {
		if !strings.Contains(demoRobotsTxt, want) {
			t.Errorf("robots.txt missing %q", want)
		}
	}
	if rec := do(http.MethodGet, "/", nil); !strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") {
		t.Errorf("static shell must stay noindex, got %q", rec.Header().Get("X-Robots-Tag"))
	}

	rec := do(http.MethodPost, "/api/v1/demo/session", nil)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Value != "" {
			cookie = c
		}
	}
	if rec.Code != http.StatusOK || cookie == nil {
		t.Fatalf("demo session status = %d body = %s", rec.Code, rec.Body.String())
	}

	tests := []struct {
		method, target string
		want           int
	}{
		{http.MethodGet, "/api/v1/apps", http.StatusOK},
		{http.MethodGet, "/api/v1/nodes", http.StatusOK},
		{http.MethodGet, "/api/v1/system/containers", http.StatusOK},
		{http.MethodPost, "/api/v1/apps", http.StatusForbidden},
		{http.MethodDelete, "/api/v1/apps/storefront-web", http.StatusForbidden},
		{http.MethodGet, "/api/v1/apps/storefront-web/terminal", http.StatusForbidden},
	}
	for _, tt := range tests {
		if got := do(tt.method, tt.target, cookie).Code; got != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.target, got, tt.want)
		}
	}
}

func TestRunDemoServeFailsClosed(t *testing.T) {
	brandFile := filepath.Join(t.TempDir(), "brand.yaml")
	if err := os.WriteFile(brandFile, []byte("name: Test\nshort_name: tp\nbinary_name: tp\ndomain: example.test\n"), 0o600); err != nil {
		t.Fatalf("write brand: %v", err)
	}
	errDockerSocket := errors.New("a Docker socket is present")
	tests := []struct {
		name string
		env  map[string]string
		want error
	}{
		{"no opt in", nil, nil},
		{"real docker host", map[string]string{"APP_DEMO_MODE": "1", "DOCKER_HOST": "unix:///var/run/docker.sock"}, nil},
		{"real data dir", map[string]string{"APP_DEMO_MODE": "1"}, demo.ErrNotDemoInstance},
		{"docker socket present", map[string]string{"APP_DEMO_MODE": "1"}, errDockerSocket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_DATA_DIR", t.TempDir())
			t.Setenv("APP_BRAND_FILE", brandFile)
			t.Setenv("APP_DEMO_MODE", "")
			t.Setenv("DOCKER_HOST", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			// Hosts with a real Docker socket must not mask the case under test.
			present := errors.Is(tt.want, errDockerSocket)
			t.Cleanup(demo.SetSocketStatForTest(func(string) (os.FileInfo, error) {
				if present {
					return os.Stat(os.DevNull)
				}
				return nil, os.ErrNotExist
			}))
			err := runDemoServe(context.Background(), demoTestLogger())
			if err == nil {
				t.Fatal("runDemoServe() = nil, want a refusal")
			}
			if errors.Is(tt.want, errDockerSocket) {
				if !strings.Contains(err.Error(), errDockerSocket.Error()) {
					t.Errorf("error = %v, want a Docker socket refusal", err)
				}
			} else if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNextDemoResetAt(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 20, 0, 0, time.UTC)
	cases := []struct {
		name, interval string
		want           time.Time
	}{
		{"unset keeps daily", "", time.Date(2026, 3, 2, 4, 0, 0, 0, time.UTC)},
		{"invalid keeps daily", "soon", time.Date(2026, 3, 2, 4, 0, 0, 0, time.UTC)},
		{"too short keeps daily", "5s", time.Date(2026, 3, 2, 4, 0, 0, 0, time.UTC)},
		{"half hour", "30m", time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)},
		{"hourly", "1h", time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := nextDemoResetAt(now, c.interval); !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDemoThemeInit_DefaultsLight(t *testing.T) {
	rec := httptest.NewRecorder()
	serveDemoThemeInit(rec, httptest.NewRequest(http.MethodGet, "/theme-init.js", nil))
	body := rec.Body.String()
	for _, want := range []string{"data-theme-default", "stored === 'dark'", "stored === 'oled'", "stored === 'system'"} {
		if !strings.Contains(body, want) {
			t.Errorf("variant missing %q", want)
		}
	}
	if strings.Contains(body, "stored !== 'light'") {
		t.Error("variant still defaults to dark")
	}
	shared, err := os.ReadFile("../../web/public/theme-init.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shared), "stored !== 'light'") {
		t.Error("shared theme-init.js must keep its dark default")
	}
}

func TestDemoHeadOptions_PublicURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if got := demoHeadOptions(logger, "").URL; got != "https://demo.levelrail.com/" {
		t.Errorf("default URL = %q", got)
	}
	o := demoHeadOptions(logger, "https://demo.example.org/some/path")
	if o.URL != "https://demo.example.org/" || o.ImageURL != "https://demo.example.org/demo-og.png" {
		t.Errorf("custom = %+v", o)
	}
	for _, bad := range []string{"http://demo.example.org", "/relative", "https://u:p@x.org"} {
		if got := demoHeadOptions(logger, bad).URL; got != "https://demo.levelrail.com/" {
			t.Errorf("%q not ignored: %q", bad, got)
		}
	}
}

func TestDemoHandlerWidenedCSPOnEveryResponse(t *testing.T) {
	ctx := context.Background()
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_DEMO_GTM_ID", "GTM-ABC123")
	t.Setenv("APP_DEMO_ANALYTICS_HOST", "https://us.i.posthog.com")
	t.Setenv("APP_DEMO_ANALYTICS_TOKEN", "phc_testtoken")
	orig := demoWebHandler
	demoWebHandler = func(b *brand.Brand, head web.HeadOptions) http.Handler {
		return web.HandlerFromFS(fstest.MapFS{
			"dist/index.html":        &fstest.MapFile{Data: []byte("<html><head></head>shell</html>")},
			"dist/assets/app-123.js": &fstest.MapFile{Data: []byte("x")},
		}, b, head)
	}
	t.Cleanup(func() { demoWebHandler = orig })

	db, err := openStore(ctx)
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := demo.Seed(ctx, db, demo.Options{Seed: 1, Anchor: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	tdb, err := openTelemetryStore(ctx)
	if err != nil {
		t.Fatalf("openTelemetryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = tdb.Close() })
	adb, err := openAlertingStore(ctx)
	if err != nil {
		t.Fatalf("openAlertingStore() error = %v", err)
	}
	t.Cleanup(func() { _ = adb.Close() })
	h, err := buildDemoHandler(ctx, demoTestLogger(), demoTestBrand(), db, tdb, adb, telemetry.NewLogBroadcaster())
	if err != nil {
		t.Fatalf("buildDemoHandler() error = %v", err)
	}

	for _, target := range []string{"/", "/login", "/assets/app-123.js", "/api/v1/demo", "/assets/missing.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		csp := rec.Header().Get("Content-Security-Policy")
		for _, want := range []string{
			"script-src 'self' https://www.googletagmanager.com https://us.i.posthog.com https://us-assets.i.posthog.com;",
			"https://us-assets.i.posthog.com; object-src",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("GET %s (status %d) CSP %q missing %q", target, rec.Code, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
			t.Errorf("GET %s CSP must not allow unsafe script: %q", target, csp)
		}
	}
}
