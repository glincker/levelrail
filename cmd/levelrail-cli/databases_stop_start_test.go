package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesStop(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db", Suspended: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "stop", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/databases/db/stop" {
		t.Errorf("request = %s %s, want POST /api/v1/databases/db/stop", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "name:     db") {
		t.Errorf("stdout = %q, want the database printed", stdout)
	}
}

func TestRun_DatabasesStop_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "stop"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}

func TestRun_DatabasesStart(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "start", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/databases/db/start" {
		t.Errorf("request = %s %s, want POST /api/v1/databases/db/start", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "name:     db") {
		t.Errorf("stdout = %q, want the database printed", stdout)
	}
}

func TestRun_DatabasesStart_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "start"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}
