package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDeploySpecFixture(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write app.yaml fixture: %v", err)
	}
	return path
}

const multiServiceYAML = `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./web/Dockerfile
    port: 3000
  worker:
    build:
      type: dockerfile
      path: ./worker/Dockerfile
    port: 4000
`

func TestRun_AppsDeploySpec(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody deploySpecRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(deploySpecResult{
			AppID: "myapp",
			Services: []deploySpecServiceResult{
				{ServiceKey: "web", ServiceName: "myapp-web", Image: "myapp-web:abc"},
				{ServiceKey: "worker", ServiceName: "myapp-worker", Image: "myapp-worker:abc"},
			},
			AllSucceeded: true,
		})
	}))
	defer srv.Close()

	file := writeDeploySpecFixture(t, multiServiceYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/myapp/deploy-spec" {
		t.Errorf("path = %q, want /api/v1/apps/myapp/deploy-spec", gotPath)
	}
	if gotBody.RepoURL != "https://github.com/you/app.git" || gotBody.Ref != "main" {
		t.Errorf("request body = %+v, want repo_url/ref set from flags", gotBody)
	}
	if len(gotBody.Services) != 2 {
		t.Fatalf("request body services = %d, want 2", len(gotBody.Services))
	}
	if gotBody.Services["web"].Build.Path != "./web/Dockerfile" {
		t.Errorf("web build path = %q, want ./web/Dockerfile", gotBody.Services["web"].Build.Path)
	}
	if !strings.Contains(stdout.String(), `"app_id": "myapp"`) {
		t.Errorf("stdout = %q, want the deploy result as JSON", stdout.String())
	}
}

func TestRun_AppsDeploySpec_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(deploySpecResult{
			AppID: "myapp",
			Services: []deploySpecServiceResult{
				{ServiceKey: "web", ServiceName: "myapp-web", Image: "myapp-web:abc"},
			},
			AllSucceeded: true,
		})
	}))
	defer srv.Close()

	file := writeDeploySpecFixture(t, multiServiceYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "app_id: myapp") {
		t.Errorf("stdout = %q, want app_id line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "myapp-web:abc") {
		t.Errorf("stdout = %q, want the built image listed", stdout.String())
	}
}

func TestRun_AppsDeploySpec_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_ = json.NewEncoder(w).Encode(deploySpecResult{
			AppID: "myapp",
			Services: []deploySpecServiceResult{
				{ServiceKey: "web", ServiceName: "myapp-web", Image: "myapp-web:abc"},
				{ServiceKey: "worker", ServiceName: "myapp-worker", Error: "build failed"},
			},
			AllSucceeded: false,
		})
	}))
	defer srv.Close()

	file := writeDeploySpecFixture(t, multiServiceYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (a partial failure), stdout=%q", got, exitAPIError, stdout.String())
	}
	if !strings.Contains(stdout.String(), "build failed") {
		t.Errorf("stdout = %q, want the failing service's error", stdout.String())
	}
}

func TestRun_AppsDeploySpec_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--repo-url", "https://github.com/you/app.git", "--ref", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--file is required") {
		t.Errorf("stderr = %q, want a missing --file validation error", stderr.String())
	}
}

func TestRun_AppsDeploySpec_MissingRepoURL(t *testing.T) {
	file := writeDeploySpecFixture(t, multiServiceYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--ref", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--repo-url is required") {
		t.Errorf("stderr = %q, want a missing --repo-url validation error", stderr.String())
	}
}

func TestRun_AppsDeploySpec_MissingRef(t *testing.T) {
	file := writeDeploySpecFixture(t, multiServiceYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--ref is required") {
		t.Errorf("stderr = %q, want a missing --ref validation error", stderr.String())
	}
}

func TestRun_AppsDeploySpec_NoServices(t *testing.T) {
	file := writeDeploySpecFixture(t, "version: 1\nservices: {}\n")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_AppsDeploySpec_SecretEnvRejected(t *testing.T) {
	yaml := `version: 1
services:
  web:
    build:
      type: dockerfile
    port: 3000
    env:
      API_KEY: { secret: true, required: true }
`
	file := writeDeploySpecFixture(t, yaml)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "secret") {
		t.Errorf("stderr = %q, want a secret-env-not-supported error", stderr.String())
	}
}

func TestRun_AppsDeploySpec_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "--file", "app.yaml"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsDeploySpec_FileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "myapp", "--file", "/nonexistent/app.yaml", "--repo-url", "https://github.com/you/app.git", "--ref", "main"}, &stdout, &stderr, envMap())
	if got != exitNetwork {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitNetwork, stderr.String())
	}
	if !strings.Contains(stderr.String(), "/nonexistent/app.yaml") {
		t.Errorf("stderr = %q, want the unreadable path named", stderr.String())
	}
}

func TestRun_AppsDeploySpec_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"invalid app spec"}`)

	file := writeDeploySpecFixture(t, multiServiceYAML)

	stderr := runCLIExpectAPIError(t, []string{"apps", "deploy-spec", "myapp", "--file", file, "--repo-url", "https://github.com/you/app.git", "--ref", "main", "--api-url", srv.URL})
	if !strings.Contains(stderr, "invalid app spec") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsDeploySpec_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-spec", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps deploy-spec") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
