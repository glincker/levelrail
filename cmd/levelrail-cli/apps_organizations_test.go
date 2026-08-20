package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsOrganizationsCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createOrganizationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(organizationResource{ID: "org_1", Name: gotBody.Name, CreatedAt: "2026-08-16T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "organizations", "create", "--name", "Acme", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/organizations" {
		t.Errorf("request = %s %s, want POST /api/v1/organizations", gotMethod, gotPath)
	}
	if gotBody.Name != "Acme" {
		t.Errorf("request body Name = %q, want Acme", gotBody.Name)
	}
	if !strings.Contains(stdout.String(), `organization "Acme" created`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsOrganizationsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "organizations", "create"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_AppsOrganizationsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []organizationResource{
		{ID: "org_1", Name: "Acme", CreatedAt: "2026-08-16T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "organizations", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/organizations" {
		t.Errorf("path = %q, want /api/v1/organizations", gotPath)
	}
	if !strings.Contains(stdout, "org_1") {
		t.Errorf("stdout = %q, want the organization id listed", stdout)
	}
}

func TestRun_AppsOrganizationsGet(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, organizationResource{ID: "org_1", Name: "Acme"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "organizations", "get", "org_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/organizations/org_1" {
		t.Errorf("path = %q, want /api/v1/organizations/org_1", gotPath)
	}
	if !strings.Contains(stdout, "Acme") {
		t.Errorf("stdout = %q, want the organization name", stdout)
	}
}

func TestRun_AppsOrganizationsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "organizations", "delete", "org_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/organizations/org_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/organizations/org_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `organization "org_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout)
	}
}

func TestRun_AppsOrganizationsSetProject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setProjectOrganizationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(projectResource{ID: "proj_1", Name: "web", OrgID: gotBody.OrgID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "organizations", "set-project", "proj_1", "org_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/projects/proj_1/organization" {
		t.Errorf("request = %s %s, want PUT /api/v1/projects/proj_1/organization", gotMethod, gotPath)
	}
	if gotBody.OrgID != "org_1" {
		t.Errorf("request body OrgID = %q, want org_1", gotBody.OrgID)
	}
	if !strings.Contains(stdout, `project "proj_1" filed under organization "org_1"`) {
		t.Errorf("stdout = %q, want an assignment confirmation", stdout)
	}
}

func TestRun_AppsOrganizationsClearProject(t *testing.T) {
	var gotBody setProjectOrganizationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(projectResource{ID: "proj_1", Name: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "organizations", "clear-project", "proj_1", "--api-url", srv.URL})
	if gotBody.OrgID != "" {
		t.Errorf("request body OrgID = %q, want empty", gotBody.OrgID)
	}
	if !strings.Contains(stdout, `project "proj_1" removed from its organization`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_AppsOrganizations_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "organizations", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps organizations") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsOrganizations_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "organizations", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps organizations subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
