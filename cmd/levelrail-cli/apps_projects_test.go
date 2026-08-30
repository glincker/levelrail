package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsProjectsCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(projectResource{ID: "proj_1", Name: gotBody.Name, CreatedAt: "2026-08-16T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects", "create", "--name", "my-saas", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/projects" {
		t.Errorf("request = %s %s, want POST /api/v1/projects", gotMethod, gotPath)
	}
	if gotBody.Name != "my-saas" {
		t.Errorf("request body Name = %q, want my-saas", gotBody.Name)
	}
	if !strings.Contains(stdout.String(), `project "my-saas" created`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsProjectsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects", "create"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_AppsProjectsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []projectResource{
		{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/projects" {
		t.Errorf("path = %q, want /api/v1/projects", gotPath)
	}
	if !strings.Contains(stdout, "proj_1") {
		t.Errorf("stdout = %q, want the project id listed", stdout)
	}
}

func TestRun_AppsProjectsGet(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, projectResource{ID: "proj_1", Name: "my-saas"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "get", "proj_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/projects/proj_1" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1", gotPath)
	}
	if !strings.Contains(stdout, "my-saas") {
		t.Errorf("stdout = %q, want the project name", stdout)
	}
}

func TestRun_AppsProjectsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "delete", "proj_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/projects/proj_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/projects/proj_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `project "proj_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout)
	}
}

func TestRun_AppsProjectsEnvGet(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, map[string]string{"LOG_LEVEL": "info"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "env-get", "proj_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/projects/proj_1/env" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1/env", gotPath)
	}
	if !strings.Contains(stdout, "LOG_LEVEL") || !strings.Contains(stdout, "info") {
		t.Errorf("stdout = %q, want the env var listed", stdout)
	}
}

func TestRun_AppsProjectsEnvGet_NoneSet(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, map[string]string{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "env-get", "proj_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no env vars set") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_AppsProjectsEnvSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "env-set", "proj_1", "--var", "LOG_LEVEL=info", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/projects/proj_1/env" {
		t.Errorf("request = %s %s, want PUT /api/v1/projects/proj_1/env", gotMethod, gotPath)
	}
	if gotBody["LOG_LEVEL"] != "info" {
		t.Errorf("request body = %+v, want LOG_LEVEL=info", gotBody)
	}
	if !strings.Contains(stdout, `project "proj_1" env vars replaced (1 set)`) {
		t.Errorf("stdout = %q, want a replace confirmation", stdout)
	}
}

func TestRun_AppsProjectsEnvSet_NoVars_ClearsAll(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "env-set", "proj_1", "--api-url", srv.URL})
	if len(gotBody) != 0 {
		t.Errorf("request body = %+v, want empty", gotBody)
	}
	if !strings.Contains(stdout, `project "proj_1" env vars replaced (0 set)`) {
		t.Errorf("stdout = %q, want a replace confirmation", stdout)
	}
}

func TestRun_AppsProjects_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps projects") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsProjects_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps projects subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsProjects_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "apps projects") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
