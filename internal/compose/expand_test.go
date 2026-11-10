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

func TestExpandBuildService_EscapingPath_Rejected(t *testing.T) {
	parent := t.TempDir()
	checkout := filepath.Join(parent, "checkout")
	if err := os.Mkdir(checkout, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "compose.yaml")
	if err := os.WriteFile(outside, []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../compose.yaml", outside, ""} {
		svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: p}}
		if _, _, err := ExpandBuildService(svc, checkout); err == nil {
			t.Errorf("ExpandBuildService(path %q) error = nil, want a rejection", p)
		}
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

func TestExpandBuildService_NonHTTPHealthcheck_BecomesExecProbe(t *testing.T) {
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
	h := out["cache"].Health
	if h == nil || h.Readiness == nil || len(h.Readiness.Exec) != 2 || h.Readiness.Exec[0] != "redis-cli" {
		t.Errorf("cache.Health = %+v, want an argv exec readiness probe", h)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
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
	// A relative bind-mount path is rejected at Parse() itself
	// (compose.Volume's own UnmarshalYAML), before ValidateForBuild ever
	// runs; see TestExpandBuildService_AbsoluteBindMountVolume_Rejected
	// for the case that reaches ValidateForBuild's own rejection.
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

// TestExpandBuildService_AbsoluteBindMountVolume_Rejected proves
// ValidateForBuild rejects a bind mount even when it's a well-formed,
// non-deny-listed absolute path, unlike Validate (the direct-import
// path, TestParse_AbsoluteBindMountVolume in compose_test.go): the
// expanded result here is a spec.Service, and spec.Volume has no
// host-path concept to carry a bind mount into (ValidateForBuild's own
// doc comment).
func TestExpandBuildService_AbsoluteBindMountVolume_Rejected(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    build: ./web
    volumes:
      - /srv/data:/data
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

func TestExpandBuildService_MagicVars(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
x-levelrail-domains:
  web: app.example.com
services:
  web:
    image: nginx
    environment:
      HOST: ${SERVICE_FQDN_WEB_3000}
      PUBLIC: ${SERVICE_URL_WEB}
      DB_URL: postgres://u:${SERVICE_PASSWORD_DB}@db/x
  db:
    image: postgres
    environment:
      POSTGRES_PASSWORD: ${SERVICE_PASSWORD_DB}
      PEER: ${SERVICE_FQDN_API}
      PEER_URL: ${SERVICE_URL_DB_5432}
`)
	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	run := func() (map[string]spec.Service, []string, map[string]string) {
		stored := map[string]string{}
		gen := func(kind, key string, _ int) (string, error) { return "pw-" + kind + "-" + key, nil }
		persist := func(s, k, v string) error { stored[s+"/"+k] = v; return nil }
		out, warns, err := ExpandBuildService(svc, sourceDir, WithMagicSecrets(gen, persist))
		if err != nil {
			t.Fatalf("ExpandBuildService() error = %v", err)
		}
		return out, warns, stored
	}
	out, warns, stored := run()
	if got := out["web"].Env["HOST"].Value; got != "app.example.com" {
		t.Errorf("HOST = %q, want the service domain", got)
	}
	if got := out["web"].Env["PUBLIC"].Value; got != "https://app.example.com" {
		t.Errorf("PUBLIC = %q", got)
	}
	if got := out["db"].Env["PEER"].Value; got != "db" || len(warns) != 2 {
		t.Errorf("PEER = %q warnings = %v, want internal name plus one warning", got, warns)
	}
	if got := out["db"].Env["PEER_URL"].Value; got != "http://db:5432" {
		t.Errorf("PEER_URL = %q, want the internal name with the token port", got)
	}
	if !out["db"].Env["POSTGRES_PASSWORD"].Secret || out["db"].Env["POSTGRES_PASSWORD"].Value != "" {
		t.Errorf("password must be secret-backed with no literal value")
	}
	if stored["db/POSTGRES_PASSWORD"] != "pw-PASSWORD-DB" || stored["web/DB_URL"] != "postgres://u:"+stored["db/POSTGRES_PASSWORD"]+"@db/x" {
		t.Errorf("stored = %v, want one shared generated password", stored)
	}
	_, _, again := run()
	if again["db/POSTGRES_PASSWORD"] != stored["db/POSTGRES_PASSWORD"] {
		t.Errorf("password changed on second expand")
	}
}

func TestExpandBuildService_MagicVar_UnknownKindStillErrors(t *testing.T) {
	sourceDir := t.TempDir()
	writeComposeFile(t, sourceDir, "docker-compose.yml", `
services:
  web:
    image: nginx
    environment:
      X: ${SERVICE_WHATEVER_KEY}
`)
	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	gen := func(string, string, int) (string, error) { return "x", nil }
	if _, _, err := ExpandBuildService(svc, sourceDir, WithMagicSecrets(gen, func(string, string, string) error { return nil })); err == nil {
		t.Fatal("ExpandBuildService() error = nil, want unknown kind rejected")
	}
}
