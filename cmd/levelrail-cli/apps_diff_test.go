package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDiffFixture(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write app.yaml fixture: %v", err)
	}
	return path
}

const diffDeployedWebYAML = `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    domains:
      - web.example.com
    env:
      LOG_LEVEL: info
`

func diffTestServer(t *testing.T, deployedYAML string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/spec" {
			t.Errorf("path = %q, want /api/v1/apps/web/spec", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deployedYAML))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsDiff_Identical(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    domains:
      - web.example.com
    env:
      LOG_LEVEL: info
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "no differences") {
		t.Errorf("stdout = %q, want a no-differences message", stdout.String())
	}
}

func TestRun_AppsDiff_ChangedFieldReportedAndNonzeroExit(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 4000
    domains:
      - web.example.com
      - new.example.com
    env:
      LOG_LEVEL: debug
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitDiffFound {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitDiffFound, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "port: deployed=3000 local=4000") {
		t.Errorf("stdout = %q, want a port change line", out)
	}
	if !strings.Contains(out, "domain added: new.example.com") {
		t.Errorf("stdout = %q, want a domain-added line", out)
	}
	if !strings.Contains(out, "env LOG_LEVEL: deployed=info local=debug") {
		t.Errorf("stdout = %q, want an env change line", out)
	}
}

func TestRun_AppsDiff_Quiet_SuppressesOutputKeepsExitCode(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 4000
    domains:
      - web.example.com
    env:
      LOG_LEVEL: info
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--quiet", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitDiffFound {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitDiffFound, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty output with --quiet", stdout.String())
	}
}

func TestRun_AppsDiff_Quiet_IdenticalStillExitsOK(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, diffDeployedWebYAML)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--quiet", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty output with --quiet", stdout.String())
	}
}

func TestRun_AppsDiff_SecretRequiredMismatchIsNotADifference(t *testing.T) {
	srv := diffTestServer(t, `version: 1
services:
  web:
    port: 3000
    env:
      API_KEY: { secret: true }
`)
	file := writeDiffFixture(t, `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    env:
      API_KEY: { secret: true, required: true }
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d: a secret's required flag is a known, non-comparable fidelity loss, not real drift (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
}

func TestRun_AppsDiff_FromReferenceIsNotComparable(t *testing.T) {
	srv := diffTestServer(t, `version: 1
services:
  web:
    port: 3000
    env:
      DATABASE_URL: postgres://resolved
`)
	file := writeDiffFixture(t, `version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    env:
      DATABASE_URL: { from: postgres.main.url }
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d: a { from: ... } reference can't be compared against an already-resolved value (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "not comparable") {
		t.Errorf("stdout = %q, want a not-comparable note for DATABASE_URL", stdout.String())
	}
}

func TestRun_AppsDiff_SingleServiceFileWithDifferentKeyMatches(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, `version: 1
services:
  frontend:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    domains:
      - web.example.com
    env:
      LOG_LEVEL: info
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d: file's sole service should be used when no key matches name (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
}

func TestRun_AppsDiff_NoMatchingServiceIsValidationError(t *testing.T) {
	srv := diffTestServer(t, diffDeployedWebYAML)
	file := writeDiffFixture(t, `version: 1
services:
  frontend:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
  worker:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 4000
`)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "diff", "web", file, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}
