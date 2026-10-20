package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsErrorsListAndGet(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		issue := apiclient.ErrorIssue{Fingerprint: "fp1", Title: "TypeError: x", Level: "error", Count: 4, Status: "open", Regressed: true, LastSeenNs: 1_700_000_000_000_000_000}
		switch {
		case r.Method == http.MethodPatch:
			issue.Status = "resolved"
			_ = json.NewEncoder(w).Encode(apiclient.ErrorIssueDetail{Issue: issue})
		case strings.HasSuffix(r.URL.Path, "/fp1"):
			ev := `{"exception":{"values":[{"type":"TypeError","value":"x","stacktrace":{"frames":[{"filename":"a.js","function":"outer","lineno":1},{"filename":"b.js","function":"inner","lineno":9}]}}]}}`
			_ = json.NewEncoder(w).Encode(apiclient.ErrorIssueDetail{Issue: issue, Events: []apiclient.ErrorEvent{{Event: json.RawMessage(ev)}}})
		default:
			_ = json.NewEncoder(w).Encode([]apiclient.ErrorIssue{issue})
		}
	}))
	defer srv.Close()
	run1 := func(args ...string) string {
		var stdout, stderr bytes.Buffer
		code := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap())
		if code != exitOK {
			t.Fatalf("%v exit = %d stderr=%q", args, code, stderr.String())
		}
		return stdout.String()
	}
	if out := run1("apps", "errors", "list", "web", "--status", "open", "--since", "24h"); !strings.Contains(out, "open (regressed)") || !strings.Contains(out, "TypeError: x") {
		t.Errorf("list output:\n%s", out)
	}
	if out := run1("apps", "errors", "get", "web", "fp1"); !strings.Contains(out, "at inner (b.js:9)") || strings.Index(out, "inner") > strings.Index(out, "outer") {
		t.Errorf("get output (innermost frame first):\n%s", out)
	}
	if out := run1("apps", "errors", "resolve", "web", "fp1"); !strings.Contains(out, "fp1 is now resolved") {
		t.Errorf("resolve output: %s", out)
	}
	if paths[0] != "GET /api/v1/apps/web/errors?since=24h&status=open" || paths[2] != "PATCH /api/v1/apps/web/errors/fp1" {
		t.Errorf("paths = %v", paths)
	}
}

func TestRun_AppsErrorTrackingEnablePrintsDSN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ErrorTrackingResource{Enabled: true, DSN: "https://k@cp.example.com/api/v1/sentry/1"})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "error-tracking", "enable", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK || !strings.Contains(stdout.String(), "SENTRY_DSN=https://k@cp.example.com/api/v1/sentry/1") {
		t.Fatalf("exit = %d out=%q err=%q", code, stdout.String(), stderr.String())
	}
}
