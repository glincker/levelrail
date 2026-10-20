package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsGitSourceSetBuildOnly(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitSourceResource{
			ServiceName: "web", BuildType: "dockerfile", BuildPath: "apps/glinr/deploy/Dockerfile",
			ResolvedBuild: apiclient.GitSourceResolvedBuild{ContextDir: ".", Summary: "Builds apps/glinr/deploy/Dockerfile with the repository root as the build context."},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "set", "web", "--build-type", "dockerfile", "--dockerfile", "apps/glinr/deploy/Dockerfile", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/git-source/build" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/git-source/build", gotMethod, gotPath)
	}
	if gotBody["build_path"] != "apps/glinr/deploy/Dockerfile" || gotBody["base_directory"] != "" {
		t.Errorf("body = %v", gotBody)
	}
	if !strings.Contains(stdout, "repository root as the build context") {
		t.Errorf("stdout = %q, want the resolved build summary", stdout)
	}
}

func TestRun_AppsGitSourceDetectApply(t *testing.T) {
	var applied map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/web/git-source/detect":
			_ = json.NewEncoder(w).Encode(apiclient.GitBuildDetection{
				Branch: "main", LooksLikeMonorepo: true, NeedsBuildSettings: true, Tools: []string{"turbo"},
				Suggestions: []apiclient.GitBuildSuggestion{{
					BuildType: "dockerfile", DockerfilePath: "apps/glinr/deploy/Dockerfile", Recommended: true,
					Reason: "Dockerfile at apps/glinr/deploy/Dockerfile; it runs turbo prune, so keep the build context at the repository root.",
				}},
			})
		case "/api/v1/apps/web/git-source/build":
			_ = json.NewDecoder(r.Body).Decode(&applied)
			_ = json.NewEncoder(w).Encode(gitSourceResource{ServiceName: "web", BuildType: "dockerfile"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "detect", "web", "--apply", "--api-url", srv.URL})
	if !strings.Contains(stdout, "turbo prune") || !strings.Contains(stdout, "apps/glinr/deploy/Dockerfile") {
		t.Errorf("stdout = %q, want the suggestion and its reason", stdout)
	}
	if applied["build_type"] != "dockerfile" || applied["build_path"] != "apps/glinr/deploy/Dockerfile" {
		t.Errorf("applied = %v, want the recommended suggestion", applied)
	}
}

func TestRun_AppsGitSourceDetectPackageAndAffected(t *testing.T) {
	var build, deploy map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/web/git-source/detect":
			_ = json.NewEncoder(w).Encode(apiclient.GitBuildDetection{
				Branch: "main", LooksLikeMonorepo: true,
				Packages: []apiclient.WorkspacePackage{{Name: "web", Dir: "apps/web", Tool: "pnpm-workspace", WatchPaths: []string{"apps/web/**", "pnpm-lock.yaml"}}},
			})
		case "/api/v1/apps/web/git-source/build":
			_ = json.NewDecoder(r.Body).Decode(&build)
			_ = json.NewEncoder(w).Encode(gitSourceResource{ServiceName: "web", BuildType: "railpack"})
		case "/api/v1/apps/web/git-source/deploy-settings":
			_ = json.NewDecoder(r.Body).Decode(&deploy)
			_ = json.NewEncoder(w).Encode(apiclient.GitDeploySettings{})
		case "/api/v1/apps/web/git-source/affected":
			_ = json.NewEncoder(w).Encode(apiclient.GitSourceAffected{
				AppDeploys: true, Packages: []apiclient.GitSourceAffectedEntry{{Dir: "apps/web", Affected: true}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "git-source", "detect", "web", "--package", "apps/web", "--api-url", srv.URL})
	if build["base_directory"] != "apps/web" {
		t.Errorf("build = %v, want base_directory apps/web", build)
	}
	if paths, _ := deploy["deploy_paths"].([]any); len(paths) != 2 {
		t.Errorf("deploy = %v, want the package watch paths", deploy)
	}
	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "affected", "web", "--files", "apps/web/a.ts", "--api-url", srv.URL})
	if !strings.Contains(stdout, "apps/web") || !strings.Contains(stdout, "affected=true") {
		t.Errorf("stdout = %q", stdout)
	}
}
