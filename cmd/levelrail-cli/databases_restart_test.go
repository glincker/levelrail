package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesRestart(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "restart", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/databases/db/restart" {
		t.Errorf("request = %s %s, want POST /api/v1/databases/db/restart", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `database "db" restarted`) {
		t.Errorf("stdout = %q, want a restart confirmation", stdout)
	}
}

func TestRun_DatabasesRestart_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restart", "db", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a non-zero exit on a server error", got)
	}
	if !strings.Contains(stderr.String(), "restart database") {
		t.Errorf("stderr = %q, want a restart-failure message", stderr.String())
	}
}

func TestRun_DatabasesRestart_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "restart"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}
