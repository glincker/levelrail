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
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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
	tests := []struct {
		name string
		env  map[string]string
		want error
	}{
		{"no opt in", nil, nil},
		{"real docker host", map[string]string{"APP_DEMO_MODE": "1", "DOCKER_HOST": "unix:///var/run/docker.sock"}, nil},
		{"real data dir", map[string]string{"APP_DEMO_MODE": "1"}, demo.ErrNotDemoInstance},
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
			err := runDemoServe(context.Background(), demoTestLogger())
			if err == nil {
				t.Fatal("runDemoServe() = nil, want a refusal")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
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
