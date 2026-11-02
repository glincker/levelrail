package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
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
