package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runVisitors(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runAppsVisitors("levelrail-cli-test", args, &out, &errBuf, envMap())
	return code, out.String(), errBuf.String()
}

func TestAppsVisitorsSendsWindowAndPrintsHistory(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.Visitors{
			App: "web", Window: "30d", Requests: 12, Visitors: 5,
			Status:     map[string]int{"2xx": 10, "5xx": 2},
			TopPaths:   []apiclient.VisitorItem{{Label: "/", Count: 9}},
			OldestKept: time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC),
		})
	}))
	t.Cleanup(srv.Close)
	code, out, errOut := runVisitors(t, "web", "--window", "30d", "--domain", "a.test", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if gotPath != "/api/v1/apps/web/visitors" || !strings.Contains(gotQuery, "window=30d") || !strings.Contains(gotQuery, "domain=a.test") {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	for _, want := range []string{"visitors 5", "requests 12", "window 30d", "data since 2026-10-05 14:00 UTC", "PAGE"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAppsVisitorsPagesOnlyPrintsClassSplit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.Visitors{
			App: "web", Window: "24h", Requests: 40, Visitors: 3, PagesOnly: true,
			Status:   map[string]int{"2xx": 40},
			TopPaths: []apiclient.VisitorItem{{Label: "/api/v1/stats", Count: 30}},
			TopPages: []apiclient.VisitorItem{{Label: "/pricing", Count: 4}},
			Classes:  apiclient.VisitorClasses{Page: 4, API: 30, Asset: 5, Bot: 1},
		})
	}))
	t.Cleanup(srv.Close)
	code, out, errOut := runVisitors(t, "web", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	for _, want := range []string{"classes  page 4, api 30, asset 5, bot 1, prefetch 0, other 0", "PAGE", "VIEWS", "/pricing"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/api/v1/stats") {
		t.Errorf("top pages must not list API paths:\n%s", out)
	}
}

func TestAppsVisitorsPagesOnlyWithoutPageViews(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.Visitors{
			App: "web", Window: "24h", Requests: 36, PagesOnly: true, Status: map[string]int{"2xx": 36},
			Classes: apiclient.VisitorClasses{API: 30, Asset: 5, Bot: 1},
		})
	}))
	t.Cleanup(srv.Close)
	_, out, _ := runVisitors(t, "web", "--api-url", srv.URL)
	if !strings.Contains(out, "No page views yet. 36 API, asset and bot requests were seen.") {
		t.Errorf("output:\n%s", out)
	}
}

func TestAppsVisitorsNeedsAnApp(t *testing.T) {
	if code, _, _ := runVisitors(t); code != exitUsage {
		t.Fatalf("exit = %d, want usage", code)
	}
}

func TestAppsVisitorsHelpNamesAllWindows(t *testing.T) {
	_, out, errOut := runVisitors(t, "-h")
	if !strings.Contains(out+errOut, "1h, 24h, 7d or 30d") {
		t.Errorf("help = %q %q", out, errOut)
	}
}

func TestAppsVisitorsExportWritesFile(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("section,key,requests,visitors,extra\n"))
	}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "v.csv")
	code, _, errOut := runVisitors(t, "web", "--export", "csv", "--window", "7d", "--out", file, "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	data, err := os.ReadFile(file) //nolint:gosec // test temp path
	if err != nil || !strings.HasPrefix(string(data), "section,key") {
		t.Fatalf("file = %q, err %v", data, err)
	}
	if !strings.Contains(gotQuery, "/visitors/export?") || !strings.Contains(gotQuery, "window=7d") {
		t.Errorf("request = %s", gotQuery)
	}
	if code, _, _ := runVisitors(t, "web", "--export", "xml", "--api-url", srv.URL); code != exitUsage {
		t.Errorf("bad format exit = %d", code)
	}
}
