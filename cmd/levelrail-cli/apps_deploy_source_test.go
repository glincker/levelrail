package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func sourceDeployServer(t *testing.T, built *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/web/git-source" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(gitSourceResource{ServiceName: "web", RepoURL: "https://github.com/org/app.git", Branch: "main", BuildType: "dockerfile", BuildPath: "deploy/Dockerfile"})
		case r.URL.Path == "/api/v1/apps/web/git-source/commits":
			_ = json.NewEncoder(w).Encode(apiclient.GitSourceCommits{
				Branch: "main", Provider: "github", UpToDate: false,
				Head:        &apiclient.GitCommit{SHA: "abcdef1234567"},
				Commits:     []apiclient.GitCommit{{SHA: "abcdef1234567", Message: "feat: x", Author: "Ann"}},
				Suggestions: []apiclient.DeploySuggestion{{ID: "add_health_check", Severity: "info", Reason: "No health check is configured."}},
			})
		case r.URL.Path == "/api/v1/apps/web/builds" && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(built)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(buildTriggerResponse{ID: "dep_1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsDeployLatestBuildsHeadWithSourceSettings(t *testing.T) {
	var built map[string]any
	srv := sourceDeployServer(t, &built)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploy", "web", "--latest", "--api-url", srv.URL})
	if built["ref"] != "abcdef1234567" || built["repo_url"] != "https://github.com/org/app.git" {
		t.Errorf("build body = %v", built)
	}
	b, _ := built["build"].(map[string]any)
	if b["type"] != "dockerfile" || b["path"] != "deploy/Dockerfile" {
		t.Errorf("build settings = %v", b)
	}
	if !strings.Contains(stdout, "dep_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsDeployCommit(t *testing.T) {
	var built map[string]any
	srv := sourceDeployServer(t, &built)
	runCLIExpectOK(t, []string{"apps", "deploy", "web", "--commit", "deadbeef", "--api-url", srv.URL})
	if built["ref"] != "deadbeef" {
		t.Errorf("ref = %v", built["ref"])
	}
}

func TestRun_AppsGitSourceCommitsHuman(t *testing.T) {
	var built map[string]any
	srv := sourceDeployServer(t, &built)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "commits", "web", "--api-url", srv.URL})
	for _, want := range []string{"abcdef1", "feat: x", "add_health_check"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}
