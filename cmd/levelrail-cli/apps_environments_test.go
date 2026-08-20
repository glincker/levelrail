package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsEnvironmentsCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createEnvironmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(environmentResource{ID: "env_1", ProjectID: "proj_1", Name: gotBody.Name})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "environments", "create", "proj_1", "--name", "staging", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/projects/proj_1/environments" {
		t.Errorf("request = %s %s, want POST /api/v1/projects/proj_1/environments", gotMethod, gotPath)
	}
	if gotBody.Name != "staging" {
		t.Errorf("request body Name = %q, want staging", gotBody.Name)
	}
	if !strings.Contains(stdout, `environment "staging" created`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout)
	}
}

func TestRun_AppsEnvironmentsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "environments", "create", "proj_1"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_AppsEnvironmentsCreate_MissingProjectID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "environments", "create", "--name", "staging"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing project id usage error", stderr.String())
	}
}

func TestRun_AppsEnvironmentsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []environmentResource{
		{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-16T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "environments", "list", "proj_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/projects/proj_1/environments" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1/environments", gotPath)
	}
	if !strings.Contains(stdout, "env_1") {
		t.Errorf("stdout = %q, want the environment id listed", stdout)
	}
}

func TestRun_AppsEnvironmentsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "environments", "delete", "env_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/environments/env_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/environments/env_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `environment "env_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout)
	}
}

func TestRun_AppsEnvironments_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "environments", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps environments") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsEnvironments_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "environments", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps environments subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
