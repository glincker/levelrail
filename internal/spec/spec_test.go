package spec

import (
	"os"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	// name is always a literal fixture filename from a test case in this
	// file, never external input.
	data, err := os.ReadFile("testdata/" + name) //nolint:gosec // fixed test fixture path, not user input
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}

func TestParse_ValidFull(t *testing.T) {
	s, err := Parse(readTestdata(t, "valid_full.yaml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}

	web, ok := s.Services["web"]
	if !ok {
		t.Fatal("expected a \"web\" service")
	}
	if web.Build.Type != BuildDockerfile || web.Build.Path != "./Dockerfile" {
		t.Errorf("Build = %+v, want type=dockerfile path=./Dockerfile", web.Build)
	}
	if len(web.Domains) != 1 || web.Domains[0] != "app.example.com" {
		t.Errorf("Domains = %v, want [app.example.com]", web.Domains)
	}
	if web.Port != 3000 {
		t.Errorf("Port = %d, want 3000", web.Port)
	}
	if web.Health == nil || web.Health.Readiness == nil || web.Health.Readiness.Path != "/healthz" {
		t.Errorf("Health.Readiness = %+v, want Path=/healthz", web.Health)
	}
	if web.Health.Liveness == nil || web.Health.Liveness.Failures != 3 {
		t.Errorf("Health.Liveness = %+v, want Failures=3", web.Health.Liveness)
	}
	if web.Resources == nil || web.Resources.Memory != "512Mi" || web.Resources.CPU != 0.5 {
		t.Errorf("Resources = %+v, want Memory=512Mi CPU=0.5", web.Resources)
	}
	if web.Replicas != 2 {
		t.Errorf("Replicas = %d, want 2", web.Replicas)
	}
	if web.Strategy != StrategyRolling {
		t.Errorf("Strategy = %q, want %q", web.Strategy, StrategyRolling)
	}

	dbURL, ok := web.Env["DATABASE_URL"]
	if !ok || dbURL.From != "postgres.main.url" || dbURL.Value != "" || dbURL.Secret {
		t.Errorf("Env[DATABASE_URL] = %+v, want From=postgres.main.url only", dbURL)
	}
	apiKey, ok := web.Env["API_KEY"]
	if !ok || !apiKey.Secret || !apiKey.Required || apiKey.From != "" || apiKey.Value != "" {
		t.Errorf("Env[API_KEY] = %+v, want Secret=true Required=true only", apiKey)
	}

	main, ok := s.Databases["main"]
	if !ok {
		t.Fatal("expected a \"main\" database")
	}
	if main.Engine != EnginePostgres || main.Version != "16" {
		t.Errorf("Databases[main] = %+v, want Engine=postgres Version=16", main)
	}
	if main.Backup == nil || main.Backup.Retain != 7 {
		t.Errorf("Databases[main].Backup = %+v, want Retain=7", main.Backup)
	}
}

func TestParse_ValidMinimal_Defaults(t *testing.T) {
	s, err := Parse(readTestdata(t, "valid_minimal.yaml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	web := s.Services["web"]
	if got := web.EffectiveReplicas(); got != DefaultReplicas {
		t.Errorf("EffectiveReplicas() = %d, want %d", got, DefaultReplicas)
	}
	if got := web.EffectiveStrategy(); got != StrategyBlueGreen {
		t.Errorf("EffectiveStrategy() = %q, want %q (documented default)", got, StrategyBlueGreen)
	}
}

func TestParse_ValidStatic_NoPortRequired(t *testing.T) {
	s, err := Parse(readTestdata(t, "valid_static.yaml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	docs := s.Services["docs"]
	if docs.Build.Type != BuildStatic {
		t.Errorf("Build.Type = %q, want %q", docs.Build.Type, BuildStatic)
	}
	if docs.Port != 0 {
		t.Errorf("Port = %d, want 0 (static sites have no container to route to)", docs.Port)
	}
}

// TestParse_Invalid covers errors from both validation layers: schema
// (structural shape) and Validate (semantic, cross-field/cross-service).
// Schema error messages come from the library and aren't asserted on
// beyond "an error occurred"; semantic error messages are ours, and
// those are checked for the specific reason, not just presence of an
// error, since a wrong reason failing silently as "some error" would
// hide a real regression.
func TestParse_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantErrSubstr string // empty means "just check it's an error" (schema-layer cases)
	}{
		{
			name: "missing version",
			yaml: `
services:
  web: { build: { type: dockerfile }, port: 8080 }
`,
		},
		{
			name: "wrong version",
			yaml: `
version: 2
services:
  web: { build: { type: dockerfile }, port: 8080 }
`,
		},
		{
			name: "no services",
			yaml: `
version: 1
services: {}
`,
		},
		{
			name: "unknown top-level field",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080 }
extra_field: true
`,
		},
		{
			name: "unknown field within a service",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080, made_up_field: 1 }
`,
		},
		{
			name: "invalid build type",
			yaml: `
version: 1
services:
  web: { build: { type: not-a-real-type }, port: 8080 }
`,
		},
		{
			name: "missing build",
			yaml: `
version: 1
services:
  web: { port: 8080 }
`,
		},
		{
			name: "invalid strategy",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080, strategy: yolo }
`,
		},
		{
			name: "port out of range",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 99999 }
`,
		},
		{
			name: "bad memory pattern",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080, resources: { memory: "lots" } }
`,
		},
		{
			name: "bad interval pattern",
			yaml: `
version: 1
services:
  web:
    build: { type: dockerfile }
    port: 8080
    health: { readiness: { path: /healthz, interval: "five seconds" } }
`,
		},
		{
			name: "compose build type missing path",
			yaml: `
version: 1
services:
  web: { build: { type: compose }, port: 8080 }
`,
			wantErrSubstr: "build.path is required for build.type: compose",
		},
		{
			name: "port required for non-static build",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile } }
`,
			wantErrSubstr: "port is required unless build.type is",
		},
		{
			name: "port must not be set for static build",
			yaml: `
version: 1
services:
  docs: { build: { type: static }, port: 8080 }
`,
			wantErrSubstr: "port must not be set when build.type is",
		},
		{
			name: "duplicate domain across services",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080, domains: [shared.example.com] }
  api: { build: { type: dockerfile }, port: 8081, domains: [shared.example.com] }
`,
			wantErrSubstr: "is claimed by both service",
		},
		{
			name: "invalid service name",
			yaml: `
version: 1
services:
  Web: { build: { type: dockerfile }, port: 8080 }
`,
			wantErrSubstr: "name must be lowercase alphanumeric",
		},
		{
			// Caught by the schema's engine enum before Validate's own
			// (currently unreachable while the two stay in sync, kept
			// anyway per the same reasoning as the strategy check in
			// Service.validate) redundant check ever runs.
			name: "unsupported database engine",
			yaml: `
version: 1
services:
  web: { build: { type: dockerfile }, port: 8080 }
databases:
  main: { engine: mysql }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("Parse() error = nil, want an error")
			}
			if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte("not: valid: yaml: at: all: [[["))
	if err == nil {
		t.Fatal("Parse() error = nil for malformed YAML, want an error")
	}
}
