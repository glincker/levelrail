package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	data, stdout := runMigrateFileMode(t, "dokploy", srv.URL, "dokploy-token", outDir, "my-app.yaml", "my-app:")
	if strings.Contains(data, "super-secret-literal-value") {
		t.Errorf("app.yaml leaked a real env value: %q", data)
	}
	if !strings.Contains(stdout, "my-app") {
		t.Errorf("stdout = %q, want the migrated app named", stdout)
	}
	if !strings.Contains(stdout, "Dokploy") {
		t.Errorf("stdout = %q, want the Dokploy source label", stdout)
	}
}

func TestRun_MigrateDokploy_Apply(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "web", SourceType: "github", BuildType: "dockerfile"}
	domains := []dokployDomain{{Host: "web.example.com", Port: 3000}}
	dokploySrv := fakeDokployServer(t, app, domains)
	defer dokploySrv.Close()

	runMigrateApply(t, "dokploy", dokploySrv.URL, "dokploy-token", "web", 3000)
}

func TestRun_MigrateDokploy_BlockingAppReported(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "image-app", SourceType: "docker", DockerImage: "ghcr.io/acme/app:latest"}
	srv := fakeDokployServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")
	runMigrateBlockingAppReported(t, "dokploy", srv.URL, outDir, "image-app")
}

func TestRun_MigrateDokploy_JSON(t *testing.T) {
	app := dokployApplication{ApplicationID: "a1", Name: "web", SourceType: "github", BuildType: "dockerfile"}
	domains := []dokployDomain{{Host: "web.example.com", Port: 3000}}
	srv := fakeDokployServer(t, app, domains)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")
	report := runMigrateJSON(t, "dokploy", srv.URL, outDir, "web")
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

func TestRun_MigrateDokploy_DryRunNeverPrintsBasicAuthPassword(t *testing.T) {
	const canary = "dry-run-canary-pw-91"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project.all":
			_, _ = w.Write([]byte(`[{"projectId":"p1","name":"default"}]`))
		case "/api/project.one":
			_, _ = w.Write([]byte(`{"projectId":"p1","applications":[{"applicationId":"a1","name":"portal"}]}`))
		case "/api/application.one":
			_, _ = w.Write([]byte(`{"applicationId":"a1","name":"portal","sourceType":"github","buildType":"dockerfile","security":[{"username":"ops","password":"` + canary + `"}]}`))
		case "/api/domain.byApplicationId":
			_, _ = w.Write([]byte(`[{"host":"portal.example.com","port":3000}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Without --apply the command is the dry run: it prints the plan and writes only app.yaml.
	for _, extra := range [][]string{{}, {"--json"}} {
		args := append([]string{"migrate", "dokploy", "--url", srv.URL, "--token", "t", "--out-dir", t.TempDir()}, extra...)
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), canary) {
			t.Fatalf("dry-run output %v leaked the basic-auth password", extra)
		}
	}
}
