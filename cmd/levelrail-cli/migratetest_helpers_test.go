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

// fakeTargetAppsServer returns an httptest.Server standing in for the
// target control plane's POST /api/v1/apps, and the appResource it decodes
// the created app into.
func fakeTargetAppsServer(t *testing.T) (*httptest.Server, *appResource) {
	t.Helper()
	created := &appResource{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps" {
			_ = json.NewDecoder(r.Body).Decode(created)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv, created
}

// runMigrateFileMode runs `migrate <source>` in file mode, asserts it
// succeeds and writes wantYAMLName declaring wantServiceDecl, and returns
// the generated file's contents plus stdout for provider-specific checks.
func runMigrateFileMode(t *testing.T, source, url, token, outDir, wantYAMLName, wantServiceDecl string) (data, stdout string) {
	t.Helper()
	var stdoutBuf, stderrBuf bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", source,
		"--url", url, "--token", token,
		"--out-dir", outDir,
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdoutBuf.String(), stderrBuf.String())
	}

	raw, err := os.ReadFile(filepath.Join(outDir, wantYAMLName)) //nolint:gosec // outDir is a t.TempDir() this test created, not user input
	if err != nil {
		t.Fatalf("expected generated app.yaml: %v", err)
	}
	if !strings.Contains(string(raw), wantServiceDecl) {
		t.Errorf("app.yaml = %q, want it to declare service %s", raw, wantServiceDecl)
	}
	if !strings.Contains(stdoutBuf.String(), "apps create --file") {
		t.Errorf("stdout = %q, want the repo/ref follow-up instruction", stdoutBuf.String())
	}
	return string(raw), stdoutBuf.String()
}

// runMigrateApply runs `migrate <source> --apply` against a fake target API
// server, and asserts the run succeeded and created wantName/wantPort.
func runMigrateApply(t *testing.T, source, url, token, wantName string, wantPort int) {
	t.Helper()
	targetSrv, created := fakeTargetAppsServer(t)
	defer targetSrv.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", source,
		"--url", url, "--token", token,
		"--apply", "--target-api-url", targetSrv.URL, "--target-token", "target-token",
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdoutBuf.String(), stderrBuf.String())
	}
	if created.Name != wantName {
		t.Errorf("created app name = %q, want %q", created.Name, wantName)
	}
	if created.Port != wantPort {
		t.Errorf("created app port = %d, want %d", created.Port, wantPort)
	}
	if !strings.Contains(stdoutBuf.String(), "applied") {
		t.Errorf("stdout = %q, want confirmation the app was applied", stdoutBuf.String())
	}
}

// runMigrateBlockingAppReported runs `migrate <source>` for an app whose
// build entirely blocks migration, and asserts it's reported for manual
// review with no app.yaml written.
func runMigrateBlockingAppReported(t *testing.T, source, url, outDir, wantAppName string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", source,
		"--url", url, "--token", "t",
		"--out-dir", outDir,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "needs manual review") {
		t.Errorf("stdout = %q, want a manual-review section", stdout.String())
	}
	if !strings.Contains(stdout.String(), wantAppName) {
		t.Errorf("stdout = %q, want the blocked app named", stdout.String())
	}
	if _, err := os.Stat(outDir); err == nil {
		if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
			t.Errorf("out-dir has entries %v, want none for an all-blocking migration", entries)
		}
	}
}

// runMigrateJSON runs `migrate <source> --json`, asserts it succeeds and
// decodes to a report with exactly one app named wantAppName.
func runMigrateJSON(t *testing.T, source, url, outDir, wantAppName string) migrationReport {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"migrate", source,
		"--url", url, "--token", "t",
		"--out-dir", outDir, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}

	var report migrationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if len(report.Apps) != 1 || report.Apps[0].ServiceName != wantAppName {
		t.Errorf("report.Apps = %+v, want one app named %s", report.Apps, wantAppName)
	}
	return report
}
