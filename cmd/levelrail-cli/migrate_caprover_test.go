package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_MigrateCaprover_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "caprover"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--url and --token") {
		t.Errorf("stderr = %q, want a missing --url/--token message", stderr.String())
	}
}

func TestRun_MigrateCaprover_WrongPassword(t *testing.T) {
	srv := fakeCaproverServer(t, "correct-password", "session-token", nil)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "caprover", "--url", srv.URL, "--token", "wrong-password",
	}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a non-zero exit for a wrong password (stdout=%q)", got, stdout.String())
	}
	if !strings.Contains(stderr.String(), "log in to caprover") {
		t.Errorf("stderr = %q, want a login failure message", stderr.String())
	}
}

// TestRun_MigrateCaprover_FileMode does not use the shared
// runMigrateFileMode helper: that helper asserts the "apps create --file"
// repo/ref follow-up is always printed, which never applies to CapRover
// since it exposes no git source (see mapCaproverApplication's doc
// comment), unlike Coolify and Dokploy.
func TestRun_MigrateCaprover_FileMode(t *testing.T) {
	app := caproverAppDefinition{
		AppName: "My App", ContainerHTTPPort: 3000,
		EnvVars: []caproverEnvVar{{Key: "API_KEY", Value: "super-secret-literal-value"}},
	}
	srv := fakeCaproverServer(t, "captain42", "session-token", []caproverAppDefinition{app})
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")

	var stdoutBuf, stderrBuf bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", "caprover",
		"--url", srv.URL, "--token", "captain42",
		"--out-dir", outDir,
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdoutBuf.String(), stderrBuf.String())
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "my-app.yaml")) //nolint:gosec // outDir is a t.TempDir() this test created, not user input
	if err != nil {
		t.Fatalf("expected generated app.yaml: %v", err)
	}
	data := string(raw)
	if !strings.Contains(data, "my-app:") {
		t.Errorf("app.yaml = %q, want it to declare service my-app", data)
	}
	if strings.Contains(data, "super-secret-literal-value") {
		t.Errorf("app.yaml leaked a real env value: %q", data)
	}

	stdout := stdoutBuf.String()
	if !strings.Contains(stdout, "my-app") {
		t.Errorf("stdout = %q, want the migrated app named", stdout)
	}
	if !strings.Contains(stdout, "CapRover") {
		t.Errorf("stdout = %q, want the CapRover source label", stdout)
	}
	if strings.Contains(stdout, "apps create --file") {
		t.Errorf("stdout = %q, want no repo/ref follow-up (CapRover exposes no git source)", stdout)
	}
}

func TestRun_MigrateCaprover_Apply(t *testing.T) {
	app := caproverAppDefinition{AppName: "web", ContainerHTTPPort: 3000}
	srv := fakeCaproverServer(t, "captain42", "session-token", []caproverAppDefinition{app})
	defer srv.Close()

	runMigrateApply(t, "caprover", srv.URL, "captain42", "web", 3000)
}

func TestRun_MigrateCaprover_JSON(t *testing.T) {
	app := caproverAppDefinition{AppName: "web", ContainerHTTPPort: 3000}
	srv := fakeCaproverServer(t, "t", "session-token", []caproverAppDefinition{app})
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "migrated")
	report := runMigrateJSON(t, "caprover", srv.URL, outDir, "web")
	if report.CaproverURL != srv.URL {
		t.Errorf("report.CaproverURL = %q, want %q", report.CaproverURL, srv.URL)
	}
}

func TestRun_MigrateCaprover_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"migrate", "caprover", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "migrate caprover") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
