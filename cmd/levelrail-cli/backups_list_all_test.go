package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsListAll(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupHistoryResource{
			{ID: "bkh_vol", ResourceKind: "volume", ServiceName: "web", VolumeName: "data", TargetID: "tgt_1", Status: "succeeded", StartedAt: "2026-08-15T00:01:00Z"},
			{ID: "bkh_db", ResourceKind: "database", DatabaseName: "main", TargetID: "tgt_1", Status: "succeeded", StartedAt: "2026-08-15T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "list-all", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/backups" {
		t.Errorf("path = %q, want /api/v1/backups", gotPath)
	}
	if !strings.Contains(stdout.String(), "bkh_vol") || !strings.Contains(stdout.String(), "bkh_db") {
		t.Errorf("stdout = %q, want both backup ids listed", stdout.String())
	}
	if !strings.Contains(stdout.String(), "web/data") {
		t.Errorf("stdout = %q, want the volume resource identifier rendered", stdout.String())
	}
}

func TestRun_BackupsListAll_Pagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupHistoryResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"backups", "list-all",
		"--limit", "2", "--before", "2026-08-15T00:00:00Z",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotQuery != "before=2026-08-15T00%3A00%3A00Z&limit=2" {
		t.Errorf("query = %q, want limit and before both forwarded", gotQuery)
	}
}

func TestRun_BackupsListAll_RejectsPositionalArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "list-all", "main"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsListAll_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupHistoryResource{
			{ID: "bkh_db", ResourceKind: "database", DatabaseName: "main", TargetID: "tgt_1", Status: "succeeded", StartedAt: "2026-08-15T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "list-all", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var decoded []backupHistoryResource
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode --json output: %v (stdout=%q)", err, stdout.String())
	}
	if len(decoded) != 1 || decoded[0].ID != "bkh_db" {
		t.Fatalf("decoded = %+v, want exactly the seeded row", decoded)
	}
}
