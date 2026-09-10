package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_BitbucketApp_Repos(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []bitbucketAppRepoResource{{FullName: "acme/widgets", DefaultBranch: "main"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"bitbucket-app", "repos", "--api-url", srv.URL})

	if gotPath != "/api/v1/bitbucket-app/repos" {
		t.Errorf("path = %s, want /api/v1/bitbucket-app/repos", gotPath)
	}
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
}

func TestRun_BitbucketApp_Branches(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitAppBranchResource{{Name: "main"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"bitbucket-app", "branches", "acme", "widgets", "--api-url", srv.URL})

	if gotPath != "/api/v1/bitbucket-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/bitbucket-app/repos/acme/widgets/branches", gotPath)
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want main listed", stdout)
	}
}

func TestRun_BitbucketApp_Branches_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"bitbucket-app", "branches", "acme", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires a workspace and a repo slug") {
		t.Errorf("stderr = %q, want a missing-arg usage error", stderr.String())
	}
}

func TestRun_BitbucketApp_UseAsSource(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, gitSourceResource{ServiceName: "web"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"bitbucket-app", "use-as-source", "acme", "widgets", "--app-name", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/bitbucket-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/bitbucket-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "web") {
		t.Errorf("stdout = %q, want web reflected", stdout)
	}
}

func TestRun_BitbucketApp_UseAsSource_MissingAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"bitbucket-app", "use-as-source", "acme", "widgets", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --app-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_BitbucketApp_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"bitbucket-app", "-h"})
	if !strings.Contains(stdout, "bitbucket-app repos") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_BitbucketApp_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"no bitbucket app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"bitbucket-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no bitbucket app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}
