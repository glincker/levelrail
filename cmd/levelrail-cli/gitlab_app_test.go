package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_GitLabApp_Projects(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitLabAppProjectResource{
		{ID: 7, PathWithNamespace: "acme/widgets", Visibility: "private", DefaultBranch: "main"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitlab-app", "projects", "--api-url", srv.URL})

	if gotPath != "/api/v1/gitlab-app/projects" {
		t.Errorf("path = %s, want /api/v1/gitlab-app/projects", gotPath)
	}
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
}

func TestRun_GitLabApp_Branches(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitAppBranchResource{{Name: "main"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitlab-app", "branches", "7", "--api-url", srv.URL})

	if gotPath != "/api/v1/gitlab-app/projects/7/branches" {
		t.Errorf("path = %s, want /api/v1/gitlab-app/projects/7/branches", gotPath)
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want main listed", stdout)
	}
}

func TestRun_GitLabApp_Branches_InvalidProjectID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"gitlab-app", "branches", "not-a-number", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "project-id must be a positive integer") {
		t.Errorf("stderr = %q, want a project-id validation error", stderr.String())
	}
}

func TestRun_GitLabApp_UseAsSource(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, gitSourceResource{ServiceName: "web"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitlab-app", "use-as-source", "7", "--app-name", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/gitlab-app/projects/7/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/gitlab-app/projects/7/use-as-source", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "service_name") && !strings.Contains(stdout, "web") {
		t.Errorf("stdout = %q, want web reflected", stdout)
	}
}

func TestRun_GitLabApp_UseAsSource_MissingAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"gitlab-app", "use-as-source", "7", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --app-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_GitLabApp_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"gitlab-app", "-h"})
	if !strings.Contains(stdout, "gitlab-app projects") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_GitLabApp_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"no gitlab app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"gitlab-app", "projects", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no gitlab app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}
