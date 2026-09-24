package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AuditLog_SearchAndFailedForwarded(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]auditLogEntryResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--search", "alice", "--failed", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(gotQuery, "q=alice") || !strings.Contains(gotQuery, "status=failed") {
		t.Errorf("query = %q, want q and status forwarded", gotQuery)
	}
}
