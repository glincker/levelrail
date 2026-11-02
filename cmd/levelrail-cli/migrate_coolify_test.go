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

	"github.com/GLINCKER/levelrail/internal/platformimport/fakecoolify"
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

	data, stdout := runMigrateFileMode(t, "coolify", srv.URL, "coolify-token", outDir, "my-app.yaml", "my-app:")
	if strings.Contains(data, "***") {
		t.Errorf("app.yaml leaked a Coolify redacted value: %q", data)
	}
	if !strings.Contains(stdout, "my-app") {
		t.Errorf("stdout = %q, want the migrated app named", stdout)
	}
}

func TestRun_MigrateCoolify_Apply(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "web", BuildPack: "dockerfile", PortsExposes: "3000"}
	coolifySrv := fakeCoolifyServer(t, app, nil)
	defer coolifySrv.Close()

	runMigrateApply(t, "coolify", coolifySrv.URL, "coolify-token", "web", 3000)
}

func TestRun_MigrateCoolify_BlockingAppReported(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "compose-app", BuildPack: "dockercompose"}
	srv := fakeCoolifyServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")
	runMigrateBlockingAppReported(t, "coolify", srv.URL, outDir, "compose-app")
}

func TestRun_MigrateCoolify_JSON(t *testing.T) {
	app := coolifyApplication{UUID: "u1", Name: "web", BuildPack: "dockerfile", PortsExposes: "3000"}
	srv := fakeCoolifyServer(t, app, nil)
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")
	runMigrateJSON(t, "coolify", srv.URL, outDir, "web")
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

func TestRun_MigrateCoolify_StagesDiscoveredService(t *testing.T) {
	base := fakecoolify.New().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/applications" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		if r.URL.Path == "/api/v1/services/"+fakecoolify.ServiceUUID {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"docker_compose_raw": "services:\n  plausible:\n    image: plausible/analytics:v2\n    environment:\n      - SERVICE_FQDN_PLAUSIBLE_8000\n      - SECRET_KEY=${SERVICE_PASSWORD_64_KEY}\n"})
			return
		}
		base.ServeHTTP(w, r)
	}))
	defer srv.Close()

	stage := filepath.Join(t.TempDir(), "compose")
	var stdout, stderr bytes.Buffer
	code := run("lr", []string{"migrate", "coolify", "--url", srv.URL, "--token", fakecoolify.Token,
		"--out-dir", t.TempDir(), "--stage-dir", stage, "--domain-suffix", "apps.test", "--json"}, &stdout, &stderr, loopbackEnv())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	path := filepath.Join(stage, "plausible", "compose.yaml")
	raw, err := os.ReadFile(path) //nolint:gosec // path is under a t.TempDir()
	if err != nil {
		t.Fatalf("service not staged: %v (stdout=%s)", err, stdout.String())
	}
	if hasRawMagicVar(string(raw)) || strings.Contains(string(raw), "SERVICE_PASSWORD") || !strings.Contains(string(raw), "plausible.apps.test") {
		t.Errorf("staged compose not translated:\n%s", raw)
	}
	if !strings.Contains(stdout.String(), "levelrail-cli compose deploy plausible -f "+path) {
		t.Errorf("report lacks the deploy command: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "SECRET_KEY") {
		t.Errorf("report leaks the compose body: %s", stdout.String())
	}
}
