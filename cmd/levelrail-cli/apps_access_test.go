package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsAccessFlagsReachTheAPI(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{"time":"2026-10-15T10:00:00Z","method":"GET","path":"/slow","status":502,"duration_ms":812,"request_id":"r1","trace_id":"t1"}]`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"apps", "access", "web", "--slow-ms", "500", "--min-status", "500", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(got, "min_ms=500") || !strings.Contains(got, "min_status=500") {
		t.Fatalf("query %q", got)
	}
	if !strings.Contains(stdout.String(), "/slow") || !strings.Contains(stdout.String(), "r1") {
		t.Fatalf("output %q", stdout.String())
	}
}
