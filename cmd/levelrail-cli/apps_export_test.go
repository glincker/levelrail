package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const exportedWebYAML = `version: 1
services:
  web:
    port: 3000
    domains:
      - web.example.com
`

func TestRun_AppsExport(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(exportedWebYAML))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outFile := filepath.Join(dir, "web.yaml")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "export", "web", outFile, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/spec" {
		t.Errorf("path = %q, want /api/v1/apps/web/spec", gotPath)
	}

	data, err := os.ReadFile(outFile) //nolint:gosec // outFile is a t.TempDir() path this test constructed, not attacker-controlled input
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != exportedWebYAML {
		t.Errorf("output file = %q, want %q", data, exportedWebYAML)
	}
}

func TestRun_AppsExport_DefaultsFilename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(exportedWebYAML))
	}))
	defer srv.Close()

	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "export", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "web.yaml")); err != nil {
		t.Errorf("web.yaml was not written: %v", err)
	}
}

func TestRun_AppsExport_RefusesToOverwriteWithoutForce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(exportedWebYAML))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outFile := filepath.Join(dir, "web.yaml")
	if err := os.WriteFile(outFile, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "export", "web", outFile, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}

	data, err := os.ReadFile(outFile) //nolint:gosec // outFile is a t.TempDir() path this test constructed, not attacker-controlled input
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "pre-existing" {
		t.Errorf("output file was overwritten: %q", data)
	}
}

func TestRun_AppsExport_ForceOverwrites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(exportedWebYAML))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outFile := filepath.Join(dir, "web.yaml")
	if err := os.WriteFile(outFile, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "export", "web", outFile, "--force", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}

	data, err := os.ReadFile(outFile) //nolint:gosec // outFile is a t.TempDir() path this test constructed, not attacker-controlled input
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != exportedWebYAML {
		t.Errorf("output file = %q, want %q", data, exportedWebYAML)
	}
}

func TestRun_AppsExport_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	outFile := filepath.Join(dir, "web.yaml")

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "export", "web", outFile, "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
	}
	if _, err := os.Stat(outFile); err == nil {
		t.Error("output file should not exist after an API error")
	}
}
