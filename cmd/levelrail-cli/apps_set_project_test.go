package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSetProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", ProjectID: gotBody.ProjectID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set-project", "web", "proj_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/project" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/project", gotMethod, gotPath)
	}
	if gotBody.ProjectID != "proj_1" {
		t.Errorf("request body ProjectID = %q, want proj_1", gotBody.ProjectID)
	}
	if !strings.Contains(stdout, `app "web" moved into project "proj_1"`) {
		t.Errorf("stdout = %q, want a move confirmation", stdout)
	}
}

func TestRun_AppsSetProject_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "set-project", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and a project id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsClearProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppProjectRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "clear-project", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/project" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/project", gotMethod, gotPath)
	}
	if gotBody.ProjectID != "" {
		t.Errorf("request body ProjectID = %q, want empty", gotBody.ProjectID)
	}
	if !strings.Contains(stdout, `app "web" removed from its project`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_AppsClearProject_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "clear-project"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing app name usage error", stderr.String())
	}
}
