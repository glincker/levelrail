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

func TestRun_AppsTracingEnablePrintsEnvOnce(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.TracingResource{
			Enabled: true, Endpoint: "https://cp.example.com/api/v1/otlp", Key: "d2Vi.abc",
			Env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://cp.example.com/api/v1/otlp", "OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer d2Vi.abc"},
		})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "tracing", "enable", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/tracing" {
		t.Errorf("%s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout.String(), "OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer d2Vi.abc") || !strings.Contains(stdout.String(), "shown once") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_AppsTracesGetPrintsATree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.TraceSpan{
			{TraceID: "t", SpanID: "a", Name: "GET /pay", StartNs: 1_000_000_000, DurationNs: 50_000_000},
			{TraceID: "t", SpanID: "b", ParentID: "a", Name: "db.query", StartNs: 1_010_000_000, DurationNs: 30_000_000, StatusCode: 2, StatusMessage: "timeout"},
		})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "traces", "get", "web", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "GET /pay  +0s  50ms") || !strings.Contains(out, "  db.query  +10ms  30ms  ERROR: timeout") {
		t.Errorf("tree output:\n%s", out)
	}
}

func TestRun_AppsTracesListFlagsReachTheAPI(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "traces", "list", "web", "--errors", "--min-ms", "250", "--limit", "5", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"errors=true", "min_ms=250", "limit=5"} {
		if !strings.Contains(rawQuery, want) {
			t.Errorf("query %q lacks %q", rawQuery, want)
		}
	}
}
