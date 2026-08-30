package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsGitSourceGet(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitSourceResource{
			ServiceName: "web",
			RepoURL:     "https://github.com/acme/web",
			Branch:      "main",
			BuildType:   "dockerfile",
			WebhookURL:  "/api/v1/webhooks/github/web",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "get", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/git-source" {
		t.Errorf("request = %s %s, want GET /api/v1/apps/web/git-source", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "https://github.com/acme/web") {
		t.Errorf("stdout = %q, want the repo URL", stdout)
	}
}

func TestRun_AppsGitSourceGet_NotConnected(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"no git source connected for this app"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "git-source", "get", "web", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no git source connected") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsGitSourceSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitSourceResource{ServiceName: "web", RepoURL: gotBody["repo_url"].(string), BuildType: "dockerfile"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "set", "web", "--repo-url", "https://github.com/acme/web", "--branch", "develop", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/git-source" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/git-source", gotMethod, gotPath)
	}
	if gotBody["repo_url"] != "https://github.com/acme/web" {
		t.Errorf("body repo_url = %v, want the repo URL", gotBody["repo_url"])
	}
	if gotBody["branch"] != "develop" {
		t.Errorf("body branch = %v, want develop", gotBody["branch"])
	}
	if !strings.Contains(stdout, "repo_url:") {
		t.Errorf("stdout = %q, want the human git source summary", stdout)
	}
}

func TestRun_AppsGitSourceSet_MissingRepoURL(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "git-source", "set", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --repo-url") {
		t.Errorf("stderr = %q, want a missing --repo-url usage error", stderr.String())
	}
}

func TestRun_AppsGitSourceDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "delete", "web", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/git-source" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web/git-source", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, "disconnected") {
		t.Errorf("stdout = %q, want a disconnected confirmation", stdout)
	}
}

func TestRun_AppsGitSource_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "git-source", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps git-source subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsGitSource_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "-h"})
	if !strings.Contains(stdout, "apps git-source") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
