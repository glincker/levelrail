package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSetEnvironment(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppEnvironmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", EnvironmentID: gotBody.EnvironmentID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set-environment", "web", "env_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/environment" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/environment", gotMethod, gotPath)
	}
	if gotBody.EnvironmentID != "env_1" {
		t.Errorf("request body EnvironmentID = %q, want env_1", gotBody.EnvironmentID)
	}
	if !strings.Contains(stdout, `app "web" tagged with environment "env_1"`) {
		t.Errorf("stdout = %q, want a tagging confirmation", stdout)
	}
}

func TestRun_AppsSetEnvironment_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "set-environment", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and an environment id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsClearEnvironment(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppEnvironmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "clear-environment", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/environment" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/environment", gotMethod, gotPath)
	}
	if gotBody.EnvironmentID != "" {
		t.Errorf("request body EnvironmentID = %q, want empty", gotBody.EnvironmentID)
	}
	if !strings.Contains(stdout, `app "web" untagged from its environment`) {
		t.Errorf("stdout = %q, want an untagging confirmation", stdout)
	}
}

func TestRun_AppsClearEnvironment_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "clear-environment"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing app name usage error", stderr.String())
	}
}
