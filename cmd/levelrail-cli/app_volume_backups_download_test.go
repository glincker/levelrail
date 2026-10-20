package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppVolumeBackupsDownload(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.RequestURI(), r.Method
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tar archive content"))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "download", "web", "data", "bkh_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/volumes/data/backups/bkh_1/download" {
		t.Errorf("request = %s %s, want GET /api/v1/apps/web/volumes/data/backups/bkh_1/download", gotMethod, gotPath)
	}
	if stdout != "tar archive content" {
		t.Errorf("stdout = %q, want the raw backup body written through unmodified", stdout)
	}
}

func TestRun_AppVolumeBackupsDownload_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"backup not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"app-volume-backups", "download", "web", "data", "bkh_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppVolumeBackupsDownload_MissingArgs(t *testing.T) {
	tests := [][]string{
		{"app-volume-backups", "download"},
		{"app-volume-backups", "download", "web"},
		{"app-volume-backups", "download", "web", "data"},
	}
	for _, args := range tests {
		var stdout, stderr strings.Builder
		got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		if got != exitUsage {
			t.Fatalf("args=%v exit = %d, want %d", args, got, exitUsage)
		}
		if !strings.Contains(stderr.String(), "requires an app name, a volume name, and a backup id") {
			t.Errorf("args=%v stderr = %q, want a missing-args usage error", args, stderr.String())
		}
	}
}

func TestRun_AppVolumeBackupsDownload_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"app-volume-backups", "download", "-h"})
	if !strings.Contains(stderr, "app-volume-backups download") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
