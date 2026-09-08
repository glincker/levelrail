package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDumpFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.sql")
	if err := os.WriteFile(path, []byte("-- fixture dump\nCREATE TABLE t (id int);\n"), 0o600); err != nil {
		t.Fatalf("write dump fixture: %v", err)
	}
	return path
}

// TestRun_DatabasesRestoreUpload exercises "databases restore-upload
// <name> <file-path>" end to end against a fake control plane, the same
// run()-plus-httptest.Server shape TestRun_AppsDeployCompose uses for its
// own raw-body upload.
func TestRun_DatabasesRestoreUpload(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(restoreHistoryResource{
			ID: "rsh_1", DatabaseName: "main", Status: "succeeded",
		})
	}))
	defer srv.Close()

	file := writeDumpFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "main", file, "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/databases/main/restore-upload" {
		t.Errorf("path = %q, want /api/v1/databases/main/restore-upload", gotPath)
	}
	if gotContentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", gotContentType)
	}
	if !strings.Contains(string(gotBody), "CREATE TABLE t") {
		t.Errorf("body = %q, want the raw dump file sent verbatim", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "rsh_1"`) {
		t.Errorf("stdout = %q, want the restore outcome as JSON", stdout.String())
	}
}

func TestRun_DatabasesRestoreUpload_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(restoreHistoryResource{
			ID: "rsh_1", DatabaseName: "main", Status: "succeeded",
		})
	}))
	defer srv.Close()

	file := writeDumpFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "main", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `database "main" restored from`) || !strings.Contains(stdout.String(), "succeeded") {
		t.Errorf("stdout = %q, want a human-readable restore confirmation", stdout.String())
	}
}

func TestRun_DatabasesRestoreUpload_FileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "main", "/nonexistent/dump.sql"}, &stdout, &stderr, envMap())
	if got != exitNetwork {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitNetwork, stderr.String())
	}
	if !strings.Contains(stderr.String(), "/nonexistent/dump.sql") {
		t.Errorf("stderr = %q, want the unreadable path named", stderr.String())
	}
}

func TestRun_DatabasesRestoreUpload_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "main"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires a database name and a dump file path") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DatabasesRestoreUpload_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"restore failed: psql: syntax error"}`))
	}))
	defer srv.Close()

	file := writeDumpFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "main", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "psql: syntax error") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_DatabasesRestoreUpload_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restore-upload", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "databases restore-upload") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
