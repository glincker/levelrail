package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Registry_Status(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registrySettingsResource{Enabled: true, Host: "registry.example", Username: "levelrail", HasCredentials: true, Status: "running"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "status", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/registry" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/registry", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:         true") || !strings.Contains(stdout, "status:          running") {
		t.Errorf("stdout = %q, want enabled and status lines", stdout)
	}
}

func TestRun_Registry_Status_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registrySettingsResource{Enabled: true, Host: "registry.example", Status: "running"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "status", "--json", "--api-url", srv.URL})

	var got registrySettingsResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if !got.Enabled || got.Status != "running" {
		t.Errorf("got = %+v, want Enabled=true Status=running", got)
	}
}

func TestRun_Registry_Status_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"registry", "status", "--api-url", srv.URL})

	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_Registry_Enable(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateRegistrySettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registrySettingsResource{Enabled: true, Host: gotBody.Host, Username: "levelrail", Password: "generated-pw", Status: "running"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "enable", "--host", "registry.example", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/registry" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/registry", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.Host != "registry.example" {
		t.Errorf("request body = %+v, want Enabled=true Host=registry.example", gotBody)
	}
	if !strings.Contains(stdout, "password:        generated-pw") {
		t.Errorf("stdout = %q, want the generated password printed once", stdout)
	}
}

func TestRun_Registry_Enable_MissingHost(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry", "enable"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "--host is required") {
		t.Errorf("stderr = %q, want a --host required message", stderr.String())
	}
}

func TestRun_Registry_Enable_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotImplemented, `{"error":"the built-in registry is not configured on this control plane (no master key set)"}`)

	stderr := runCLIExpectAPIError(t, []string{"registry", "enable", "--host", "registry.example", "--api-url", srv.URL})

	if !strings.Contains(stderr, "no master key set") {
		t.Errorf("stderr = %q, want the server's configuration error", stderr)
	}
}

func TestRun_Registry_Disable(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registrySettingsResource{Enabled: false, Status: "stopped"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "disable", "--api-url", srv.URL})

	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/registry" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/registry", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:         false") {
		t.Errorf("stdout = %q, want enabled: false", stdout)
	}
}

func TestRun_Registry_Repositories(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryRepositoriesResource{Repositories: []string{"alpha", "beta"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "repositories", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/registry/repositories" {
		t.Errorf("method/path = %s %s, want GET /api/v1/registry/repositories", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "alpha") || !strings.Contains(stdout, "beta") {
		t.Errorf("stdout = %q, want both repository names", stdout)
	}
}

func TestRun_Registry_Repositories_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryRepositoriesResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "repositories", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no repositories") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_Registry_Repositories_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryRepositoriesResource{Repositories: []string{"alpha"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "repositories", "--json", "--api-url", srv.URL})
	var got registryRepositoriesResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != "alpha" {
		t.Errorf("got = %+v, want Repositories=[alpha]", got)
	}
}

func TestRun_Registry_Tags(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.Query().Get("repository")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryTagsResource{Repository: "myapp", Tags: []string{"latest", "v1"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry", "tags", "--repository", "myapp", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/registry/tags" || gotQuery != "myapp" {
		t.Errorf("method/path/query = %s %s %q, want GET /api/v1/registry/tags myapp", gotMethod, gotPath, gotQuery)
	}
	if !strings.Contains(stdout, "latest") || !strings.Contains(stdout, "v1") {
		t.Errorf("stdout = %q, want both tags", stdout)
	}
}

func TestRun_Registry_Tags_MissingRepository(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry", "tags"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "--repository is required") {
		t.Errorf("stderr = %q, want a --repository required message", stderr.String())
	}
}

func TestRun_Registry_Tags_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"repository not found in the built-in registry"}`)

	stderr := runCLIExpectAPIError(t, []string{"registry", "tags", "--repository", "ghost", "--api-url", srv.URL})

	if !strings.Contains(stderr, "repository not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_Registry_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"registry", "-h"})
	if !strings.Contains(stdout, "registry enable") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_Registry_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_Registry_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
