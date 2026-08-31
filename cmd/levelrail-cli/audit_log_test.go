package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_AuditLog(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]auditLogEntryResource{
			{ID: "aud_1", ActorType: "session", ActorName: "admin", Ability: "write", Method: "POST", Path: "/api/v1/apps", StatusCode: 201, CreatedAt: "2026-08-15T00:00:00.000000000Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/audit-log" {
		t.Errorf("path = %q, want /api/v1/audit-log", gotPath)
	}
	if !strings.Contains(stdout.String(), "aud_1") {
		t.Errorf("stdout = %q, want the entry id listed", stdout.String())
	}
}

func TestRun_AuditLog_FiltersForwarded(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]auditLogEntryResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"audit-log",
		"--limit", "2", "--before", "2026-08-15T00:00:00Z",
		"--path", "/api/v1/apps/web", "--method", "PUT",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(gotQuery, "limit=2") || !strings.Contains(gotQuery, "method=PUT") || !strings.Contains(gotQuery, "before=") {
		t.Errorf("query = %q, want limit/before/path/method all forwarded", gotQuery)
	}
}

func TestRun_AuditLog_ExportCSV_Stdout(t *testing.T) {
	const csvBody = "id,actor_type,actor_id,actor_name,ability,method,path,status_code,remote_addr,created_at\naud_1,session,user_1,admin,write,POST,/api/v1/apps,201,127.0.0.1,2026-08-15T00:00:00.000000000Z\n"
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(csvBody))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--format", "csv", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(gotQuery, "format=csv") {
		t.Errorf("query = %q, want format=csv", gotQuery)
	}
	if stdout.String() != csvBody {
		t.Errorf("stdout = %q, want the raw csv body %q", stdout.String(), csvBody)
	}
}

func TestRun_AuditLog_ExportCSV_OutputFile(t *testing.T) {
	const csvBody = "id,actor_type,actor_id,actor_name,ability,method,path,status_code,remote_addr,created_at\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(csvBody))
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "audit-log.csv")
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--format", "csv", "--output", outPath, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(outPath) //nolint:gosec // outPath is a t.TempDir() path this test itself constructed, not attacker-controlled input
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != csvBody {
		t.Errorf("file contents = %q, want %q", string(data), csvBody)
	}
}

func TestRun_AuditLog_UnsupportedFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--format", "xml"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unsupported --format") {
		t.Errorf("stderr = %q, want an unsupported-format error", stderr.String())
	}
}

func TestRun_AuditLog_OutputWithoutCSVFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--output", "out.csv"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "--output requires --format csv") {
		t.Errorf("stderr = %q, want an --output-requires-csv error", stderr.String())
	}
}

func TestRun_AuditLog_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"audit-log", "-h"})
	if !strings.Contains(stderr, "audit-log [flags]") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
