package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeComposeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte("services:\n  web:\n    image: nginx:latest\n"), 0o600); err != nil {
		t.Fatalf("write compose fixture: %v", err)
	}
	return path
}

// TestRun_AppsDeployCompose exercises "apps deploy-compose <name> --file
// <path>" end to end against a fake control plane, the same
// run()-plus-httptest.Server shape apps_restart_test.go uses.
func TestRun_AppsDeployCompose(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(composeDeployResult{
			AppID:    "myapp",
			Services: []appResource{{Name: "web", Image: "nginx:latest", Port: 80}},
		})
	}))
	defer srv.Close()

	file := writeComposeFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "myapp", "--file", file, "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/myapp/compose" {
		t.Errorf("path = %q, want /api/v1/apps/myapp/compose", gotPath)
	}
	if gotContentType != "text/yaml" {
		t.Errorf("Content-Type = %q, want text/yaml", gotContentType)
	}
	if !strings.Contains(string(gotBody), "nginx:latest") {
		t.Errorf("body = %q, want raw compose YAML sent verbatim", gotBody)
	}
	if !strings.Contains(stdout.String(), `"app_id": "myapp"`) {
		t.Errorf("stdout = %q, want the deploy result as JSON", stdout.String())
	}
}

func TestRun_AppsDeployCompose_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(composeDeployResult{
			AppID: "myapp",
			Services: []appResource{
				{Name: "web", Image: "nginx:latest", Port: 80},
				{Name: "worker", Image: "myapp/worker:latest"},
			},
		})
	}))
	defer srv.Close()

	file := writeComposeFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "myapp", "--file", file, "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "app_id: myapp") {
		t.Errorf("stdout = %q, want app_id line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "web") || !strings.Contains(stdout.String(), "nginx:latest") {
		t.Errorf("stdout = %q, want the web service listed", stdout.String())
	}
	if !strings.Contains(stdout.String(), "worker") {
		t.Errorf("stdout = %q, want the worker service listed", stdout.String())
	}
}

func TestRun_AppsDeployCompose_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "myapp"}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--file is required") {
		t.Errorf("stderr = %q, want a missing --file validation error", stderr.String())
	}
}

func TestRun_AppsDeployCompose_FileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "myapp", "--file", "/nonexistent/compose.yaml"}, &stdout, &stderr, envMap(nil))
	if got != exitNetwork {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitNetwork, stderr.String())
	}
	if !strings.Contains(stderr.String(), "/nonexistent/compose.yaml") {
		t.Errorf("stderr = %q, want the unreadable path named", stderr.String())
	}
}

func TestRun_AppsDeployCompose_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "--file", "compose.yaml"}, &stdout, &stderr, envMap(nil))
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsDeployCompose_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid compose file"}`))
	}))
	defer srv.Close()

	file := writeComposeFixture(t)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "myapp", "--file", file, "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid compose file") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsDeployCompose_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy-compose", "-h"}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps deploy-compose") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
