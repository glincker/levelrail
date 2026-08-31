package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func writeComposeFile(t *testing.T, dir, relPath, contents string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}

func TestExpandBuildService_MixedBuildAndImage(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    build:
      context: ./web
      dockerfile: Dockerfile.prod
    ports:
      - "8080:3000"
    environment:
      NODE_ENV: production
    volumes:
      - web-data:/data
    labels:
      team: platform
  redis:
    image: redis:7
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}

	web, ok := out["web"]
	if !ok {
		t.Fatal("expected service web")
	}
	if web.Build.Type != spec.BuildDockerfile {
		t.Errorf("web.Build.Type = %q, want dockerfile", web.Build.Type)
	}
	if web.Build.BaseDirectory != "web" {
		t.Errorf("web.Build.BaseDirectory = %q, want web", web.Build.BaseDirectory)
	}
	if web.Build.Path != "Dockerfile.prod" {
		t.Errorf("web.Build.Path = %q, want Dockerfile.prod", web.Build.Path)
	}
	if web.Port != 3000 {
		t.Errorf("web.Port = %d, want 3000", web.Port)
	}
	if web.Env["NODE_ENV"].Value != "production" {
		t.Errorf("web.Env[NODE_ENV] = %+v, want Value=production", web.Env["NODE_ENV"])
	}
	if len(web.Volumes) != 1 || web.Volumes[0].Name != "web-data" || web.Volumes[0].Path != "/data" {
		t.Errorf("web.Volumes = %+v, want [{web-data /data}]", web.Volumes)
	}
	if web.Labels["team"] != "platform" {
		t.Errorf("web.Labels[team] = %q, want platform", web.Labels["team"])
	}

	redis, ok := out["redis"]
	if !ok {
		t.Fatal("expected service redis")
	}
	if redis.Build.Type != spec.BuildImage || redis.Build.Image != "redis:7" {
		t.Errorf("redis.Build = %+v, want {Type:image Image:redis:7}", redis.Build)
	}
}

func TestExpandBuildService_ShortFormBuild_DefaultsContextAndDockerfile(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  api:
    build: ./api
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	api := out["api"]
	if api.Build.BaseDirectory != "api" {
		t.Errorf("api.Build.BaseDirectory = %q, want api", api.Build.BaseDirectory)
	}
	if api.Build.Path != "" {
		t.Errorf("api.Build.Path = %q, want empty (defaults to <context>/Dockerfile downstream)", api.Build.Path)
	}
}

func TestExpandBuildService_BuildWithNoContext_DefaultsToComposeFileDir(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  api:
    build:
      dockerfile: Dockerfile
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	api := out["api"]
	if api.Build.BaseDirectory != "." {
		t.Errorf("api.Build.BaseDirectory = %q, want . (compose file's own directory)", api.Build.BaseDirectory)
	}
}

func TestExpandBuildService_ComposeFileNotAtRepoRoot_ContextResolvesRelativeToComposeFile(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "services/api/docker-compose.yml", `
services:
  api:
    build: ./app
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "services/api/docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	api := out["api"]
	want := filepath.Join("services/api", "app")
	if api.Build.BaseDirectory != want {
		t.Errorf("api.Build.BaseDirectory = %q, want %q (relative to the compose file's own directory, not sourceDir)", api.Build.BaseDirectory, want)
	}
}

func TestExpandBuildService_DomainPropagates(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
x-levelrail-domains:
  web: app.example.com
services:
  web:
    build: ./web
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	if got := out["web"].Domains; len(got) != 1 || got[0] != "app.example.com" {
		t.Errorf("web.Domains = %v, want [app.example.com]", got)
	}
}

func TestExpandBuildService_TranslatableHealthcheck_PopulatesReadiness(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    build: ./web
    healthcheck:
      test: curl -f http://localhost:3000/healthz
      interval: 20s
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, warnings, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	web := out["web"]
	if web.Health == nil || web.Health.Readiness == nil {
		t.Fatal("web.Health.Readiness = nil, want a populated readiness probe")
	}
	if web.Health.Readiness.Path != "/healthz" {
		t.Errorf("web.Health.Readiness.Path = %q, want /healthz", web.Health.Readiness.Path)
	}
	if web.Health.Readiness.Interval != "20s" {
		t.Errorf("web.Health.Readiness.Interval = %q, want 20s", web.Health.Readiness.Interval)
	}
}

func TestExpandBuildService_NonHTTPHealthcheck_LeavesHealthUnsetAndWarns(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  cache:
    image: redis:7
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, warnings, err := ExpandBuildService(svc, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	if out["cache"].Health != nil {
		t.Errorf("cache.Health = %+v, want nil (no fabricated check)", out["cache"].Health)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
}

func TestExpandBuildService_MagicVar_Rejected(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    build: ./web
    environment:
      DB_PASSWORD: ${SERVICE_PASSWORD_DB}
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	_, _, err := ExpandBuildService(svc, sourceDir)
	if err == nil {
		t.Fatal("ExpandBuildService() error = nil, want a magic-var rejection error")
	}
}

func TestExpandBuildService_BindMountVolume_Rejected(t *testing.T) {
	// ValidateForBuild must still reject everything Validate itself
	// rejects other than build:, e.g. a bind-mount volume: allowing
	// build: is the one, narrow carve-out, not a general loosening.
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    build: ./web
    volumes:
      - ./local:/data
`)

	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	_, _, err := ExpandBuildService(svc, sourceDir)
	if err == nil {
		t.Fatal("ExpandBuildService() error = nil, want a bind-mount rejection error")
	}
}

func TestExpandBuildService_WrongBuildType_ReturnsCallerError(t *testing.T) {
	svc := spec.Service{Build: spec.Build{Type: spec.BuildDockerfile}}
	if _, _, err := ExpandBuildService(svc, t.TempDir()); err == nil {
		t.Fatal("ExpandBuildService() error = nil, want an error for a non-compose build type")
	}
}

func TestExpandBuildService_MissingFile_ReturnsClearError(t *testing.T) {
	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "does-not-exist.yml"}}
	if _, _, err := ExpandBuildService(svc, t.TempDir()); err == nil {
		t.Fatal("ExpandBuildService() error = nil, want a file-not-found error")
	}
}

// Validate (the direct-import path) must still reject build: even
// though ValidateForBuild now allows it: a regression check that the
// two entry points genuinely diverged, not that Validate quietly
// stopped enforcing its own rule.
func TestValidate_StillRejectsBuild(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    build: ./web
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a rejection for build:")
	}
	if err := f.ValidateForBuild(); err != nil {
		t.Errorf("ValidateForBuild() error = %v, want nil", err)
	}
}
