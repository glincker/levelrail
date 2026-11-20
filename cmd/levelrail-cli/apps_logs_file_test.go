package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func logsSearchServer(t *testing.T, gotTo *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotTo = r.URL.Query().Get("to")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entries":[{"timestamp":"2026-11-20T10:00:00Z","stream":"stdout","message":"hello"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsLogs_UntilAndFile(t *testing.T) {
	var gotTo string
	srv := logsSearchServer(t, &gotTo)
	out := filepath.Join(t.TempDir(), "app.log")

	stdout, _ := runCLIExpectOK(t, []string{"apps", "logs", "web", "--since", "1h", "--until", "2026-11-20T11:00:00Z", "--file", out, "--api-url", srv.URL})
	if gotTo != "2026-11-20T11:00:00Z" {
		t.Errorf("to = %q, want the --until value", gotTo)
	}
	if strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, entries should go to the file", stdout)
	}
	data, err := os.ReadFile(out) //nolint:gosec // temp dir path from the test
	if err != nil || !strings.Contains(string(data), "hello") {
		t.Errorf("file = %q, err %v, want the entry", data, err)
	}
	if info, _ := os.Stat(out); info == nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info)
	}
}
