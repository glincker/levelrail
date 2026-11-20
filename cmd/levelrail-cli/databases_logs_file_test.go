package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_DatabasesLogs_FileDownloadsText(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("2026-11-20T10:00:00Z stdout checkpoint complete\n"))
	}))
	t.Cleanup(srv.Close)
	out := filepath.Join(t.TempDir(), "db.log")

	runCLIExpectOK(t, []string{"databases", "logs", "main", "--since", "1h", "--file", out, "--api-url", srv.URL})
	if gotPath != "/api/v1/databases/main/logs/download" {
		t.Errorf("path = %q, want the download route", gotPath)
	}
	data, err := os.ReadFile(out) //nolint:gosec // temp dir path from the test
	if err != nil || !strings.Contains(string(data), "checkpoint complete") {
		t.Errorf("file = %q, err %v, want the exported line", data, err)
	}
	if info, _ := os.Stat(out); info == nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info)
	}
}
