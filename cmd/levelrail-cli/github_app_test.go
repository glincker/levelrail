package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_GitHubApp_Repos(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitHubAppRepoResource{
		{FullName: "acme/widgets", DefaultBranch: "main"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "repos", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app/repos" {
		t.Errorf("path = %s, want /api/v1/github-app/repos", gotPath)
	}
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
}

func TestRun_GitHubApp_Repos_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []gitHubAppRepoResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no repositories") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_GitHubApp_Repos_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"no github app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"github-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no github app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_GitHubApp_Branches(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitAppBranchResource{{Name: "main", CommitSHA: "abc123"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "branches", "acme", "widgets", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/github-app/repos/acme/widgets/branches", gotPath)
	}
	if !strings.Contains(stdout, "main") || !strings.Contains(stdout, "abc123") {
		t.Errorf("stdout = %q, want main/abc123 listed", stdout)
	}
}

func TestRun_GitHubApp_Branches_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "branches", "acme", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an owner and a repo") {
		t.Errorf("stderr = %q, want a missing-arg usage error", stderr.String())
	}
}

func TestRun_GitHubApp_UseAsSource(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody useRepoAsSourceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(useGitHubRepoAsSourceResponse{
			GitSourceResource: gitSourceResource{ServiceName: gotBody.AppName, RepoURL: "https://github.com/acme/widgets.git"},
			WebhookRegistered: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "use-as-source", "acme", "widgets", "--app-name", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/github-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/github-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if gotBody.AppName != "web" {
		t.Errorf("request body = %+v, want AppName=web", gotBody)
	}
	if !strings.Contains(stdout, "webhook_registered: true") {
		t.Errorf("stdout = %q, want webhook_registered: true", stdout)
	}
}

func TestRun_GitHubApp_UseAsSource_MissingAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "use-as-source", "acme", "widgets", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --app-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_GitHubApp_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"github-app", "-h"})
	if !strings.Contains(stdout, "github-app repos") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_GitHubApp_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown github-app subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
