package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsDownload(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.RequestURI(), r.Method
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pg_dump binary content"))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "download", "mydb", "bkh_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/databases/mydb/backups/bkh_1/download" {
		t.Errorf("request = %s %s, want GET /api/v1/databases/mydb/backups/bkh_1/download", gotMethod, gotPath)
	}
	if stdout != "pg_dump binary content" {
		t.Errorf("stdout = %q, want the raw backup body written through unmodified", stdout)
	}
}

func TestRun_BackupsDownload_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"backup not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"backups", "download", "mydb", "bkh_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_BackupsDownload_MissingArgs(t *testing.T) {
	tests := [][]string{
		{"backups", "download"},
		{"backups", "download", "mydb"},
	}
	for _, args := range tests {
		var stdout, stderr strings.Builder
		got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		if got != exitUsage {
			t.Fatalf("args=%v exit = %d, want %d", args, got, exitUsage)
		}
		if !strings.Contains(stderr.String(), "requires a database name and a backup id") {
			t.Errorf("args=%v stderr = %q, want a missing-args usage error", args, stderr.String())
		}
	}
}

func TestRun_BackupsDownload_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"backups", "download", "-h"})
	if !strings.Contains(stderr, "backups download") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
