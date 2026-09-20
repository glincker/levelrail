package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SharedEnvList(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sharedEnvVarResource{
			{Key: "LOG_LEVEL", Value: "info", Secret: false},
			{Key: "API_KEY", Value: "", Secret: true},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"shared-env", "list", "--scope", "project", "--id", "proj_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/projects/proj_1/env/all" {
		t.Errorf("request = %s %s, want GET /api/v1/projects/proj_1/env/all", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "LOG_LEVEL") || !strings.Contains(stdout, "info") {
		t.Errorf("stdout = %q, want the plain var listed with its value", stdout)
	}
	if !strings.Contains(stdout, "API_KEY") || !strings.Contains(stdout, "hidden") {
		t.Errorf("stdout = %q, want the secret var listed with a hidden value", stdout)
	}
}

func TestRun_SharedEnvList_InvalidScope(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"shared-env", "list", "--scope", "bogus", "--id", "x"}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want non-zero for an invalid --scope", got)
	}
	if !strings.Contains(stderr.String(), "--scope must be one of") {
		t.Errorf("stderr = %q, want a --scope validation error", stderr.String())
	}
}

func TestRun_SharedEnvList_MissingID(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"shared-env", "list", "--scope", "project"}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want non-zero for a missing --id", got)
	}
	if !strings.Contains(stderr.String(), "--id is required") {
		t.Errorf("stderr = %q, want an --id validation error", stderr.String())
	}
}

func TestRun_SharedEnvSet_Secret(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"shared-env", "set", "--scope", "organization", "--id", "org_1", "API_KEY", "s3cr3t", "--secret", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/organizations/org_1/env/secrets/API_KEY" {
		t.Errorf("request = %s %s, want PUT /api/v1/organizations/org_1/env/secrets/API_KEY", gotMethod, gotPath)
	}
	if gotBody["value"] != "s3cr3t" {
		t.Errorf("body value = %v, want s3cr3t", gotBody["value"])
	}
	if !strings.Contains(stdout, `secret shared var "API_KEY" set`) {
		t.Errorf("stdout = %q, want a set confirmation", stdout)
	}
}

func TestRun_SharedEnvSet_Plain_ReadsMergesWrites(t *testing.T) {
	var requests []string
	var putBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"EXISTING": "1"})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(putBody)
		}
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"shared-env", "set", "--scope", "environment", "--id", "env_1", "NEW_KEY", "new-value", "--api-url", srv.URL})
	if len(requests) != 2 || requests[0] != "GET /api/v1/environments/env_1/env" || requests[1] != "PUT /api/v1/environments/env_1/env" {
		t.Errorf("requests = %v, want a GET read followed by a PUT write, both to .../env", requests)
	}
	if putBody["EXISTING"] != "1" || putBody["NEW_KEY"] != "new-value" {
		t.Errorf("PUT body = %v, want the existing var preserved alongside the new one", putBody)
	}
	if !strings.Contains(stdout, `shared var "NEW_KEY" set`) {
		t.Errorf("stdout = %q, want a set confirmation", stdout)
	}
}

func TestRun_SharedEnvSet_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"shared-env", "set", "--scope", "project", "--id", "p1", "ONLY_KEY"}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want non-zero for a missing value argument", got)
	}
}

func TestRun_SharedEnvDelete_Secret(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"shared-env", "delete", "--scope", "project", "--id", "proj_1", "API_KEY", "--secret", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/projects/proj_1/env/secrets/API_KEY" {
		t.Errorf("request = %s %s, want DELETE /api/v1/projects/proj_1/env/secrets/API_KEY", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `secret shared var "API_KEY" removed`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_SharedEnvDelete_Plain_ReadsMergesWrites(t *testing.T) {
	var requests []string
	var putBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"KEEP": "1", "REMOVE_ME": "2"})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(putBody)
		}
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"shared-env", "delete", "--scope", "project", "--id", "proj_1", "REMOVE_ME", "--api-url", srv.URL})
	if _, stillThere := putBody["REMOVE_ME"]; stillThere {
		t.Errorf("PUT body = %v, want REMOVE_ME dropped", putBody)
	}
	if putBody["KEEP"] != "1" {
		t.Errorf("PUT body = %v, want KEEP preserved", putBody)
	}
}

func TestRun_SharedEnv_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"shared-env", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Errorf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown shared-env subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand message", stderr.String())
	}
}

func TestRun_SharedEnv_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"shared-env", "-h"})
	if !strings.Contains(stdout, "shared-env list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
