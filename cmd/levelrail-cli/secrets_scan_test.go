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

func TestRun_SecretsScanLocalFindsAndExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	tok := "gh" + "p_" + strings.Repeat("aB3", 12)
	if err := os.WriteFile(filepath.Join(dir, "app.env"), []byte("TOKEN="+tok+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"secrets", "scan", "--path", dir}, &out, &errb, envMap())
	if code != exitCheckFailed {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitCheckFailed, errb.String())
	}
	if !strings.Contains(out.String(), "github-token") || !strings.Contains(out.String(), "app.env:1") {
		t.Errorf("stdout = %q, want rule and location", out.String())
	}
	if strings.Contains(out.String(), tok) {
		t.Errorf("stdout leaked the secret value")
	}
}

func TestRun_SecretsScanLocalCleanExitsZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _ := runCLIExpectOK(t, []string{"secrets", "scan", "--path", dir})
	if !strings.Contains(stdout, "allow") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_SecretsScanAllowNeedsReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	defer srv.Close()
	runCLIExpectValidationError(t, []string{"secrets", "scan", "web", "--allow", "0123456789abcdef", "--api-url", srv.URL})
}

func TestRun_SecretsScanSetsPolicy(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, body["policy"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"app":"web","policy":"block","allow_list":[],"limits":{}}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"secrets", "scan", "web", "--policy", "block", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/secret-scan" || gotBody != "block" {
		t.Errorf("request = %s %s %q", gotMethod, gotPath, gotBody)
	}
	if !strings.Contains(stdout, "block") {
		t.Errorf("stdout = %q", stdout)
	}
}
