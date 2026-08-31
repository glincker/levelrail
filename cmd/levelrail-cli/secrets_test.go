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
	"time"
)

func TestRun_SecretsRotateMasterKey_MissingFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"secrets", "rotate-master-key"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--new-key-file is required") {
		t.Errorf("stderr = %q, want a missing-flag validation error", stderr.String())
	}
}

func TestRun_SecretsRotateMasterKey_FileNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"secrets", "rotate-master-key", "--new-key-file", "/nonexistent/path/master.key"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_SecretsRotateMasterKey_Success(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	rotatedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rotateMasterKeyResult{RotatedAt: rotatedAt, PersistedToFile: true})
	}))
	defer srv.Close()

	keyPath := filepath.Join(t.TempDir(), "new.key")
	if err := os.WriteFile(keyPath, []byte("AGE-SECRET-KEY-FAKE\n"), 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"secrets", "rotate-master-key", "--new-key-file", keyPath, "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/system/master-key/rotate" {
		t.Errorf("request = %s %s, want POST /api/v1/system/master-key/rotate", gotMethod, gotPath)
	}
	if gotBody["newMasterKey"] != "AGE-SECRET-KEY-FAKE" {
		t.Errorf("request body newMasterKey = %v, want the trimmed file contents", gotBody["newMasterKey"])
	}
	if !strings.Contains(stdout, "persisted_to_file: true") {
		t.Errorf("stdout = %q, want persisted_to_file: true", stdout)
	}
	if strings.Contains(stdout, "AGE-SECRET-KEY-FAKE") {
		t.Errorf("stdout = %q, must never print the master key material", stdout)
	}
}

func TestRun_SecretsRotateMasterKey_WarningSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rotateMasterKeyResult{
			RotatedAt: time.Now(),
			Warning:   "update APP_MASTER_KEY before the next restart",
		})
	}))
	defer srv.Close()

	keyPath := filepath.Join(t.TempDir(), "new.key")
	if err := os.WriteFile(keyPath, []byte("AGE-SECRET-KEY-FAKE"), 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"secrets", "rotate-master-key", "--new-key-file", keyPath, "--api-url", srv.URL})
	if !strings.Contains(stdout, "WARNING") || !strings.Contains(stdout, "APP_MASTER_KEY") {
		t.Errorf("stdout = %q, want the rotation warning surfaced", stdout)
	}
}

func TestRun_SecretsRotateMasterKey_JSON(t *testing.T) {
	rotatedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rotateMasterKeyResult{RotatedAt: rotatedAt, PersistedToFile: true})
	}))
	defer srv.Close()

	keyPath := filepath.Join(t.TempDir(), "new.key")
	if err := os.WriteFile(keyPath, []byte("AGE-SECRET-KEY-FAKE"), 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"secrets", "rotate-master-key", "--new-key-file", keyPath, "--api-url", srv.URL, "--json"})

	var got rotateMasterKeyResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode --json stdout: %v (stdout=%q)", err, stdout)
	}
	if !got.RotatedAt.Equal(rotatedAt) || !got.PersistedToFile {
		t.Errorf("got = %+v, want RotatedAt=%v PersistedToFile=true", got, rotatedAt)
	}
}

func TestRun_SecretsRotateMasterKey_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"unwrap DEK for \"web\": wrong key"}`)

	keyPath := filepath.Join(t.TempDir(), "new.key")
	if err := os.WriteFile(keyPath, []byte("AGE-SECRET-KEY-FAKE"), 0o600); err != nil {
		t.Fatalf("write fixture key file: %v", err)
	}

	stderr := runCLIExpectAPIError(t, []string{"secrets", "rotate-master-key", "--new-key-file", keyPath, "--api-url", srv.URL})
	if !strings.Contains(stderr, "wrong key") {
		t.Errorf("stderr = %q, want the API's own error surfaced", stderr)
	}
}

func TestRun_SecretsRotateMasterKey_StdinInput(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rotateMasterKeyResult{RotatedAt: time.Now(), PersistedToFile: true})
	}))
	defer srv.Close()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	if _, err := w.WriteString("AGE-SECRET-KEY-FROM-STDIN\n"); err != nil {
		t.Fatalf("write to pipe: %v", err)
	}
	_ = w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	runCLIExpectOK(t, []string{"secrets", "rotate-master-key", "--new-key-file", "-", "--api-url", srv.URL})

	if gotBody["newMasterKey"] != "AGE-SECRET-KEY-FROM-STDIN" {
		t.Errorf("request body newMasterKey = %v, want the trimmed stdin contents", gotBody["newMasterKey"])
	}
}

func TestRun_SecretsUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"secrets", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown secrets subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_SecretsHelp(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"secrets", "-h"})
	if !strings.Contains(stdout, "rotate-master-key") {
		t.Errorf("stdout = %q, want the rotate-master-key usage line", stdout)
	}
}
