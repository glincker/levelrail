package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsConnect(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appConnectionResource{
			EnvVar: "MAIN_DATABASE_URL", DatabaseName: gotBody["database"], Field: "url",
			Host: "main.levelrail", MeshDNS: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "connect", "web", "main", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/connections" {
		t.Errorf("method/path = %s %s, want POST /api/v1/apps/web/connections", gotMethod, gotPath)
	}
	if gotBody["database"] != "main" {
		t.Errorf("request body = %+v, want database=main", gotBody)
	}
	if !strings.Contains(stdout, "env_var:       MAIN_DATABASE_URL") {
		t.Errorf("stdout = %q, want env_var line", stdout)
	}
	if !strings.Contains(stdout, "mesh_dns=true") {
		t.Errorf("stdout = %q, want mesh_dns=true", stdout)
	}
}

func TestRun_AppsConnect_WithFieldAndEnvVar(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appConnectionResource{
			EnvVar: gotBody["env_var"], DatabaseName: gotBody["database"], Field: gotBody["field"],
		})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "connect", "web", "main", "--field", "host", "--env-var", "PRIMARY_DB_HOST", "--api-url", srv.URL})

	if gotBody["field"] != "host" || gotBody["env_var"] != "PRIMARY_DB_HOST" {
		t.Errorf("request body = %+v, want field=host env_var=PRIMARY_DB_HOST", gotBody)
	}
}

func TestRun_AppsConnect_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "connect", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsConnect_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"unknown database"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "connect", "web", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "unknown database") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_AppsDisconnect(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "disconnect", "web", "DATABASE_URL", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/connections/DATABASE_URL" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/connections/DATABASE_URL", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"disconnected": true`) && !strings.Contains(stdout, "disconnected from app") {
		t.Errorf("stdout = %q, want a disconnect confirmation", stdout)
	}
}

func TestRun_AppsConnections_List(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appConnectionResource{
			{EnvVar: "DATABASE_URL", DatabaseName: "main", Field: "url", Host: "db-main", MeshDNS: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "connections", "list", "web", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web/connections" {
		t.Errorf("path = %s, want /api/v1/apps/web/connections", gotPath)
	}
	if !strings.Contains(stdout, "DATABASE_URL") || !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want a table row for the connection", stdout)
	}
}

func TestRun_AppsConnections_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appConnectionResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "connections", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no database connections") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_AppsConnections_Suggest(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]connectableDatabaseResource{
			{Name: "cache", Engine: "redis", CrossNode: true, AlreadyConnected: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "connections", "suggest", "web", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web/connectable-databases" {
		t.Errorf("path = %s, want /api/v1/apps/web/connectable-databases", gotPath)
	}
	if !strings.Contains(stdout, "cache") || !strings.Contains(stdout, "redis") {
		t.Errorf("stdout = %q, want a table row for the candidate", stdout)
	}
}

func TestRun_AppsConnections_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "connections", "-h"})
	if !strings.Contains(stdout, "apps connections list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsConnections_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "connections", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps connections subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
