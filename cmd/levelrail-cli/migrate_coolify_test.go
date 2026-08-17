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

// fakeCoolifyServer returns an httptest.Server that answers the three
// Coolify endpoints this command calls, for one application.
func fakeCoolifyServer(t *testing.T, app coolifyApplication, envs []coolifyEnvVar) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/applications":
			_ = json.NewEncoder(w).Encode([]coolifyApplication{app})
		case "/api/v1/applications/" + app.UUID:
			_ = json.NewEncoder(w).Encode(app)
		case "/api/v1/applications/" + app.UUID + "/envs":
			_ = json.NewEncoder(w).Encode(envs)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRun_MigrateCoolify_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "coolify"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--url and --token") {
		t.Errorf("stderr = %q, want a missing --url/--token message", stderr.String())
	}
}

func TestRun_MigrateCoolify_FileMode(t *testing.T) {
	app := coolifyApplication{
		UUID: "u1", Name: "My App", BuildPack: "dockerfile",
		FQDN: "https://app.example.com", PortsExposes: "3000",
		GitRepository: "https://github.com/example/app.git", GitBranch: "main",
	}
	envs := []coolifyEnvVar{{Key: "API_KEY", Value: "***"}}
	srv := fakeCoolifyServer(t, app, envs)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "coolify",
		"--url", srv.URL, "--token", "coolify-token",
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
	if strings.Contains(string(data), "***") {
		t.Errorf("app.yaml leaked a Coolify redacted value: %q", data)
	}

	if !strings.Contains(stdout.String(), "my-app") {
		t.Errorf("stdout = %q, want the migrated app named", stdout.String())
	}
	if !strings.Contains(stdout.String(), "apps create --file") {
		t.Errorf("stdout = %q, want the repo/ref follow-up instruction", stdout.String())
	}
}

func TestRun_MigrateCoolify_Apply(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "web", BuildPack: "dockerfile", PortsExposes: "3000"}
	coolifySrv := fakeCoolifyServer(t, app, nil)
	defer coolifySrv.Close()

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
		"migrate", "coolify",
		"--url", coolifySrv.URL, "--token", "coolify-token",
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

func TestRun_MigrateCoolify_BlockingAppReported(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "compose-app", BuildPack: "dockercompose"}
	srv := fakeCoolifyServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "coolify",
		"--url", srv.URL, "--token", "t",
		"--out-dir", outDir,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "needs manual review") {
		t.Errorf("stdout = %q, want a manual-review section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "compose-app") {
		t.Errorf("stdout = %q, want the blocked app named", stdout.String())
	}
	if _, err := os.Stat(outDir); err == nil {
		if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
			t.Errorf("out-dir has entries %v, want none for an all-blocking migration", entries)
		}
	}
}

func TestRun_MigrateCoolify_JSON(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "web", BuildPack: "dockerfile", PortsExposes: "3000"}
	srv := fakeCoolifyServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "coolify",
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
}

func TestRun_MigrateCoolify_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "coolify", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "migrate coolify") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestRun_Migrate_UnknownSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown migrate source") {
		t.Errorf("stderr = %q, want an unknown-source message", stderr.String())
	}
}
