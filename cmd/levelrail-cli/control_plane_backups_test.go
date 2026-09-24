package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newControlPlaneBackupsServer(t *testing.T) *httptest.Server {
	t.Helper()
	const name = "levelrail-20260101T000000Z.db"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/system/backups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"` + name + `","size_bytes":4096,"created_at":"2026-01-01T00:00:00Z","sha256":"abc"}]`))
	})
	mux.HandleFunc("POST /api/v1/system/backups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"` + name + `","size_bytes":4096,"created_at":"2026-01-01T00:00:00Z","sha256":"abc"}`))
	})
	mux.HandleFunc("GET /api/v1/system/backups/"+name+"/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("SQLite format 3\x00"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_ControlPlaneBackups_ListCreate(t *testing.T) {
	srv := newControlPlaneBackupsServer(t)

	stdout, _ := runCLIExpectOK(t, []string{"control-plane-backups", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "levelrail-20260101T000000Z.db") || !strings.Contains(stdout, "4096") {
		t.Errorf("list stdout = %q", stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"control-plane-backups", "create", "--api-url", srv.URL})
	if !strings.Contains(stdout, "created levelrail-20260101T000000Z.db") {
		t.Errorf("create stdout = %q", stdout)
	}
}

func TestRun_ControlPlaneBackups_DownloadOut(t *testing.T) {
	srv := newControlPlaneBackupsServer(t)
	out := filepath.Join(t.TempDir(), "snap.db")

	runCLIExpectOK(t, []string{"control-plane-backups", "download", "levelrail-20260101T000000Z.db", "--out", out, "--api-url", srv.URL})
	got, err := os.ReadFile(out) //nolint:gosec // test temp path
	if err != nil || !strings.HasPrefix(string(got), "SQLite format 3") {
		t.Fatalf("saved file = %q, err = %v", got, err)
	}
}

func TestRun_ControlPlaneBackups_Delete_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	if got := run("levelrail-cli-test", []string{"control-plane-backups", "delete"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
