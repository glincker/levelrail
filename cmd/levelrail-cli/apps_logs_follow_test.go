package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseLogTestServer starts a server that answers any request on
// .../logs/stream with rawBody (already SSE-framed), records the request
// path, and closes the connection once rawBody is flushed: the same
// finite-then-EOF shape internal/apiclient's own sseServer test helper
// uses, letting runAppsLogsFollow's loop finish deterministically without
// depending on a real Ctrl+C.
func sseLogTestServer(t *testing.T, gotPath *string, rawBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test ResponseWriter does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, rawBody)
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsLogs_Follow(t *testing.T) {
	var gotPath string
	srv := sseLogTestServer(t, &gotPath,
		`data: {"line":"starting up","stream":"stdout"}`+"\n\n"+
			`data: {"line":"boom","stream":"stderr"}`+"\n\n")

	stdout, _ := runCLIExpectOK(t, []string{"apps", "logs", "web", "--follow", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/logs/stream" {
		t.Errorf("path = %q, want /api/v1/apps/web/logs/stream", gotPath)
	}
	if !strings.Contains(stdout, "stdout starting up") {
		t.Errorf("stdout = %q, want the stdout line", stdout)
	}
	if !strings.Contains(stdout, "stderr boom") {
		t.Errorf("stdout = %q, want the stderr line", stdout)
	}
}

func TestRun_AppsLogs_Follow_Short(t *testing.T) {
	var gotPath string
	srv := sseLogTestServer(t, &gotPath, `data: {"line":"hi","stream":"stdout"}`+"\n\n")

	stdout, _ := runCLIExpectOK(t, []string{"apps", "logs", "web", "-f", "--api-url", srv.URL})
	if !strings.Contains(stdout, "stdout hi") {
		t.Errorf("stdout = %q, want the streamed line", stdout)
	}
}

func TestRun_AppsLogs_Follow_JSON(t *testing.T) {
	var gotPath string
	srv := sseLogTestServer(t, &gotPath, `data: {"line":"hi","stream":"stdout"}`+"\n\n")

	stdout, _ := runCLIExpectOK(t, []string{"apps", "logs", "web", "--follow", "--json", "--api-url", srv.URL})
	if !strings.Contains(stdout, `"line":"hi"`) || !strings.Contains(stdout, `"stream":"stdout"`) {
		t.Errorf("stdout = %q, want one JSON-Lines object per entry", stdout)
	}
}

func TestRun_AppsLogs_Follow_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "logs", "missing", "--follow", "--api-url", srv.URL})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsLogs_Follow_ConflictsWithSince(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "logs", "web", "--follow", "--since", "1h"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a --follow/--since conflict error", stderr.String())
	}
}

func TestRun_AppsLogs_Follow_ConflictsWithTail(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "logs", "web", "--follow", "--tail", "10"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a --follow/--tail conflict error", stderr.String())
	}
}

func TestRun_AppsLogs_Follow_ConflictsWithQuery(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "logs", "web", "--follow", "--query", "[0]"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a --follow/--query conflict error", stderr.String())
	}
}
