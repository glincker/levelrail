package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeValidateFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestRun_AppsValidate exercises "apps validate --file <path>" end to
// end, purely local: no httptest.Server, since this command never makes
// an API call.
func TestRun_AppsValidate(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantExit       int
		wantStdout     string
		wantStderrHas  string
		wantStdoutOmit string
	}{
		{
			name: "valid app.yaml",
			content: `version: 1
services:
  web:
    build: { type: dockerfile }
    port: 8080
`,
			wantExit:   exitOK,
			wantStdout: "valid app.yaml, 1 service(s)",
		},
		{
			name: "valid compose file",
			content: `services:
  web:
    image: nginx:latest
  db:
    image: postgres:16
`,
			wantExit:   exitOK,
			wantStdout: "valid compose, 2 service(s)",
		},
		{
			name: "compose file with an unsupported deploy key fails clearly",
			content: `services:
  web:
    image: nginx:latest
    deploy:
      restart_policy:
        condition: on-failure
`,
			wantExit:      exitValidation,
			wantStderrHas: "deploy.restart_policy is not supported",
		},
		{
			name: "compose file with deploy.replicas validates cleanly",
			content: `services:
  web:
    image: nginx:latest
    deploy:
      replicas: 3
`,
			wantExit:   exitOK,
			wantStdout: "valid compose, 1 service(s)",
		},
		{
			name: "compose file with a top-level secrets block fails clearly",
			content: `services:
  web:
    image: nginx:latest
secrets:
  api_key:
    external: true
`,
			wantExit:      exitValidation,
			wantStderrHas: "secrets: is not supported yet",
		},
		{
			name:          "neither app.yaml nor compose",
			content:       "not: [valid",
			wantExit:      exitValidation,
			wantStderrHas: "spec:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeValidateFixture(t, "spec.yaml", tt.content)
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "validate", "--file", file}, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderrHas != "" && !strings.Contains(stderr.String(), tt.wantStderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderrHas)
			}
		})
	}
}

func TestRun_AppsValidate_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "validate"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--file is required") {
		t.Errorf("stderr = %q, want it to mention --file is required", stderr.String())
	}
}

// TestRun_AppsValidate_ComposeDependsOn exercises the depends_on
// reference and cycle checks through the CLI, not just internal/compose
// directly, so a regression there is caught at the same layer an
// operator would hit it.
func TestRun_AppsValidate_ComposeDependsOn(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantExit      int
		wantStderrHas string
		wantStdout    string
	}{
		{
			name: "valid depends_on",
			content: `services:
  web:
    image: nginx:latest
    depends_on: [db]
  db:
    image: postgres:16
`,
			wantExit:   exitOK,
			wantStdout: "valid compose, 2 service(s)",
		},
		{
			name: "depends_on references an unknown service",
			content: `services:
  web:
    image: nginx:latest
    depends_on: [db]
`,
			wantExit:      exitValidation,
			wantStderrHas: `references "db", which is not a service in this file`,
		},
		{
			name: "depends_on cycle",
			content: `services:
  web:
    image: nginx:latest
    depends_on: [worker]
  worker:
    image: nginx:latest
    depends_on: [web]
`,
			wantExit:      exitValidation,
			wantStderrHas: "depends_on cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := writeValidateFixture(t, "compose.yaml", tt.content)
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "validate", "--file", file}, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStderrHas != "" && !strings.Contains(stderr.String(), tt.wantStderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderrHas)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}
