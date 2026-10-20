package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesSetProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDatabaseProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db", ProjectID: gotBody.ProjectID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "set-project", "db", "proj_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/project" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/db/project", gotMethod, gotPath)
	}
	if gotBody.ProjectID != "proj_1" {
		t.Errorf("request body ProjectID = %q, want proj_1", gotBody.ProjectID)
	}
	if !strings.Contains(stdout, `database "db" moved into project "proj_1"`) {
		t.Errorf("stdout = %q, want a move confirmation", stdout)
	}
}

func TestRun_DatabasesSetProject_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "set-project", "db"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires a database name and a project id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DatabasesClearProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDatabaseProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "clear-project", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/project" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/db/project", gotMethod, gotPath)
	}
	if gotBody.ProjectID != "" {
		t.Errorf("request body ProjectID = %q, want empty", gotBody.ProjectID)
	}
	if !strings.Contains(stdout, `database "db" removed from its project`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_DatabasesClearProject_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "clear-project"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing database name usage error", stderr.String())
	}
}
