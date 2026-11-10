package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsGitSourceSetDeployOnImageApp(t *testing.T) {
	var paths []string
	var setBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&setBody)
			_ = json.NewEncoder(w).Encode(gitSourceResource{ServiceName: "web", RepoURL: "https://github.com/acme/web", Branch: "main", BuildType: "railpack"})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.GitSourceDeployResult{App: "web", Branch: "main", Started: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "set", "web", "--repo", "https://github.com/acme/web", "--branch", "main", "--build-pack", "railpack", "--deploy", "--api-url", srv.URL})
	if strings.Join(paths, ",") != "PUT /api/v1/apps/web/git-source,POST /api/v1/apps/web/git-source/deploy" {
		t.Fatalf("requests = %v", paths)
	}
	if setBody["build_type"] != "railpack" || setBody["repo_url"] != "https://github.com/acme/web" {
		t.Fatalf("body = %v", setBody)
	}
	if !strings.Contains(stdout, "build started") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_AppsGitSourceDeployJSON(t *testing.T) {
	var gotConn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/plan") {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotConn = body["connection_id"]
			_ = json.NewEncoder(w).Encode(apiclient.GitSourcePlan{App: "web", CurrentSource: "image", Group: []apiclient.GitSourcePlanMember{{Name: "worker", CurrentSource: "image", Effect: "unchanged"}}})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.GitSourceDeployResult{App: "web", Branch: "main", Started: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "deploy", "web", "--json", "--api-url", srv.URL})
	var res apiclient.GitSourceDeployResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || !res.Started || res.Branch != "main" {
		t.Fatalf("stdout is not the deploy document: %q (%v)", stdout, err)
	}
	stdout, _ = runCLIExpectOK(t, []string{"apps", "git-source", "plan", "web", "--repo", "https://gitlab.com/o/p", "--connection", "gc_1", "--api-url", srv.URL})
	if gotConn != "gc_1" || !strings.Contains(stdout, "also in app:    worker (image): unchanged") {
		t.Fatalf("connection %q, stdout %s", gotConn, stdout)
	}
}

func TestRun_AppsGitSourcePlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.GitSourcePlan{App: "site", CurrentSource: "upload", Static: true, Branch: "main",
			Suggestion: &apiclient.GitSourcePlanSuggestion{BuildType: "static", BuildCommand: "npm run build", PublishDir: "dist"}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "plan", "site", "--repo", "https://github.com/acme/site", "--api-url", srv.URL})
	for _, want := range []string{"current source: upload", "build_type=static", "--build-pack static"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}
