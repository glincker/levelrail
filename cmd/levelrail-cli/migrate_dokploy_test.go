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

// fakeDokployServer returns an httptest.Server that answers the Dokploy
// endpoints this command calls, for one project holding one application.
func fakeDokployServer(t *testing.T, app dokployApplication, domains []dokployDomain) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project.all":
			_ = json.NewEncoder(w).Encode([]dokployProject{{ProjectID: "p1", Name: "default"}})
		case "/api/project.one":
			_ = json.NewEncoder(w).Encode(dokployProjectDetail{
				ProjectID:    "p1",
				Applications: []dokployApplicationSummary{{ApplicationID: app.ApplicationID, Name: app.Name}},
			})
		case "/api/application.one":
			_ = json.NewEncoder(w).Encode(app)
		case "/api/domain.byApplicationId":
			_ = json.NewEncoder(w).Encode(domains)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRun_MigrateDokploy_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "dokploy"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--url and --token") {
		t.Errorf("stderr = %q, want a missing --url/--token message", stderr.String())
	}
}

func TestRun_MigrateDokploy_FileMode(t *testing.T) {
	app := dokployApplication{
		ApplicationID: "a1", Name: "My App", SourceType: "github", BuildType: "dockerfile",
		Owner: "acme", Repository: "my-app", Branch: "main",
		Env: "API_KEY=super-secret-literal-value\n",
	}
	domains := []dokployDomain{{Host: "app.example.com", Port: 3000}}
	srv := fakeDokployServer(t, app, domains)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "dokploy",
		"--url", srv.URL, "--token", "dokploy-token",
		"--out-dir", outDir,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(outDir, "my-app.yaml")) //nolint:gosec // outDir is a t.TempDir() this test created, not user input
	if err != nil {
		t.Fatalf("expected generated app.yaml: %v", err)
	}
	if !strings.Contains(string(data), "my-app:") {
		t.Errorf("app.yaml = %q, want it to declare service my-app", data)
	}
	if strings.Contains(string(data), "super-secret-literal-value") {
		t.Errorf("app.yaml leaked a real env value: %q", data)
	}

	if !strings.Contains(stdout.String(), "my-app") {
		t.Errorf("stdout = %q, want the migrated app named", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Dokploy") {
		t.Errorf("stdout = %q, want the Dokploy source label", stdout.String())
	}
	if !strings.Contains(stdout.String(), "apps create --file") {
		t.Errorf("stdout = %q, want the repo/ref follow-up instruction", stdout.String())
	}
}

func TestRun_MigrateDokploy_Apply(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "web", SourceType: "github", BuildType: "dockerfile"}
	domains := []dokployDomain{{Host: "web.example.com", Port: 3000}}
	dokploySrv := fakeDokployServer(t, app, domains)
	defer dokploySrv.Close()

	var createdBody appResource
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps" {
			_ = json.NewDecoder(r.Body).Decode(&createdBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(createdBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer targetSrv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "dokploy",
		"--url", dokploySrv.URL, "--token", "dokploy-token",
		"--apply", "--target-api-url", targetSrv.URL, "--target-token", "target-token",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if createdBody.Name != "web" {
		t.Errorf("created app name = %q, want %q", createdBody.Name, "web")
	}
	if createdBody.Port != 3000 {
		t.Errorf("created app port = %d, want 3000", createdBody.Port)
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Errorf("stdout = %q, want confirmation the app was applied", stdout.String())
	}
}

func TestRun_MigrateDokploy_BlockingAppReported(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "image-app", SourceType: "docker", DockerImage: "ghcr.io/acme/app:latest"}
	srv := fakeDokployServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "dokploy",
		"--url", srv.URL, "--token", "t",
		"--out-dir", outDir,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "needs manual review") {
		t.Errorf("stdout = %q, want a manual-review section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "image-app") {
		t.Errorf("stdout = %q, want the blocked app named", stdout.String())
	}
	if _, err := os.Stat(outDir); err == nil {
		if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
			t.Errorf("out-dir has entries %v, want none for an all-blocking migration", entries)
		}
	}
}

func TestRun_MigrateDokploy_JSON(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "web", SourceType: "github", BuildType: "dockerfile"}
	domains := []dokployDomain{{Host: "web.example.com", Port: 3000}}
	srv := fakeDokployServer(t, app, domains)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "dokploy",
		"--url", srv.URL, "--token", "t",
		"--out-dir", outDir, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}

	var report migrationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if len(report.Apps) != 1 || report.Apps[0].ServiceName != "web" {
		t.Errorf("report.Apps = %+v, want one app named web", report.Apps)
	}
	if report.DokployURL != srv.URL {
		t.Errorf("report.DokployURL = %q, want %q", report.DokployURL, srv.URL)
	}
}

func TestRun_MigrateDokploy_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "dokploy", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "migrate dokploy") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
