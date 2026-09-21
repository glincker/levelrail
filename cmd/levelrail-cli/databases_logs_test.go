package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesLogs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entries":[{"message":"listening on 5432"}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "logs", "db", "--api-url", srv.URL})
	if gotPath != "/api/v1/databases/db/logs" {
		t.Errorf("path = %q, want /api/v1/databases/db/logs", gotPath)
	}
	if !strings.Contains(stdout, "listening on 5432") {
		t.Errorf("stdout = %q, want the log entry", stdout)
	}
}

func TestRun_DatabasesLogs_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"database not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"databases", "logs", "missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "database not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_DatabasesLogs_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "logs"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one database name") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DatabasesLogs_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "logs", "-h"})
	if !strings.Contains(stderr, "databases logs") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_DatabasesLogs_Follow(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test ResponseWriter does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, `data: {"line":"checkpoint complete","stream":"stdout"}`+"\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "logs", "db", "--follow", "--api-url", srv.URL})
	if gotPath != "/api/v1/databases/db/logs/stream" {
		t.Errorf("path = %q, want /api/v1/databases/db/logs/stream", gotPath)
	}
	if !strings.Contains(stdout, "stdout checkpoint complete") {
		t.Errorf("stdout = %q, want the streamed line", stdout)
	}
}

func TestRun_DatabasesLogs_Follow_ConflictsWithSince(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "logs", "db", "--follow", "--since", "1h"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a --follow/--since conflict error", stderr.String())
	}
}
