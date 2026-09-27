package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runInitCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run("acme-cli", append([]string{"init"}, args...), &stdout, &stderr, envMap())
	return code, stdout.String(), stderr.String()
}

func readProjectFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInit_WritesThreeFiles(t *testing.T) {
	dir := writeProject(t, map[string]string{"go.mod": "module x\n"})
	code, stdout, stderr := runInitCLI(t, "--dir", dir, "--yes", "--api-url", "https://deploy.example.com", "--json")
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var res initResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if !res.Written || res.Stack.Provider != "golang" || len(res.Files) != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
	mcp := readProjectFile(t, dir, ".mcp.json")
	for _, want := range []string{`"acme-mcp"`, "${APP_API_TOKEN}", "agent-core", "deploy.example.com"} {
		if !strings.Contains(mcp, want) {
			t.Errorf(".mcp.json missing %q:\n%s", want, mcp)
		}
	}
	agents := readProjectFile(t, dir, "AGENTS.md")
	if !strings.Contains(agents, "acme-cli apps wait") {
		t.Errorf("AGENTS.md should use the CLI name from os.Args[0]:\n%s", agents)
	}
}

func TestInit_DryRunWritesNothing(t *testing.T) {
	dir := writeProject(t, map[string]string{"Dockerfile": "FROM alpine\nEXPOSE 80\n"})
	code, stdout, _ := runInitCLI(t, "--dir", dir, "--dry-run")
	if code != exitOK || !strings.Contains(stdout, "Dry run") {
		t.Fatalf("exit %d, stdout: %s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.yaml")); !os.IsNotExist(err) {
		t.Error("dry run must not write app.yaml")
	}
}

func TestInit_ExistingFileNeedsForce(t *testing.T) {
	dir := writeProject(t, map[string]string{"Dockerfile": "FROM alpine\nEXPOSE 80\n", "app.yaml": "version: 1\n"})
	code, stdout, stderr := runInitCLI(t, "--dir", dir, "--yes", "--json")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var res initResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	spec := res.Files[0]
	if spec.Action != initSkip || !strings.Contains(spec.Diff, "+ services:") {
		t.Errorf("existing app.yaml should be skipped with a diff, got %+v", spec)
	}
	if got := readProjectFile(t, dir, "app.yaml"); got != "version: 1\n" {
		t.Error("app.yaml was overwritten without --force")
	}

	code, _, stderr = runInitCLI(t, "--dir", dir, "--yes", "--force")
	if code != exitOK {
		t.Fatalf("force: exit %d: %s", code, stderr)
	}
	if got := readProjectFile(t, dir, "app.yaml"); !strings.Contains(got, "type: dockerfile") {
		t.Errorf("--force should overwrite app.yaml, got %s", got)
	}
}

func TestInit_NonTerminalWithoutYesFails(t *testing.T) {
	dir := writeProject(t, map[string]string{"go.mod": "module x\n"})
	code, _, stderr := runInitCLI(t, "--dir", dir)
	if code != exitValidation || !strings.Contains(stderr, "--yes") {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
}

func TestInit_BadModeAndNoStack(t *testing.T) {
	dir := writeProject(t, map[string]string{"README.md": "x"})
	if code, _, _ := runInitCLI(t, "--dir", dir, "--mode", "nope", "--yes"); code != exitValidation {
		t.Errorf("bad mode exit = %d", code)
	}
	code, stdout, _ := runInitCLI(t, "--dir", dir, "--yes", "--json")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var res initResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	if res.Files[0].Action != initSkip || res.Files[1].Action != initCreate {
		t.Errorf("no stack: want app.yaml skipped and the agent files created, got %+v", res.Files)
	}
}

func TestInitNames_BrandEnvOverrides(t *testing.T) {
	env := func(k string) (string, bool) {
		return map[string]string{"APP_BRAND_NAME": "Acme Deploy", "APP_BRAND_BINARY_NAME": "acme"}[k], k == "APP_BRAND_NAME" || k == "APP_BRAND_BINARY_NAME"
	}
	n := initNames("acme-cli", "", env)
	if n.Product != "Acme Deploy" || n.MCPBinary != "acme-mcp" || n.CLI != "acme-cli" {
		t.Errorf("names = %+v", n)
	}
}
