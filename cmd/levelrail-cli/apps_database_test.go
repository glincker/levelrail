package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDatabase_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appDatabaseResource{
			AppName: "web", DatabaseName: gotBody["database_name"], EnvVar: "DATABASE_URL", Field: "url",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "database", "set", "web", "--database-name", "main", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/database" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/database", gotMethod, gotPath)
	}
	if gotBody["database_name"] != "main" {
		t.Errorf("request body = %+v, want database_name=main", gotBody)
	}
	if !strings.Contains(stdout, "database_name: main") {
		t.Errorf("stdout = %q, want database_name line", stdout)
	}
}

func TestRun_AppsDatabase_Set_MissingDatabaseName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "database", "set", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --database-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsDatabase_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"unknown database_name"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "database", "set", "web", "--database-name", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "unknown database_name") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_AppsDatabase_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "database", "clear", "web", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/database" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/database", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "database detached") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsDatabase_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "database", "-h"})
	if !strings.Contains(stdout, "apps database set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsDatabase_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "database", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps database subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
