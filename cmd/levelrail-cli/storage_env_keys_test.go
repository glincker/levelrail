package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_StorageEnvKeysList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []string{"S3_ENDPOINT", "S3_BUCKET"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"storage-env-keys", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/storage-env-keys" {
		t.Errorf("path = %q, want /api/v1/storage-env-keys", gotPath)
	}
	if !strings.Contains(stdout, "S3_ENDPOINT") || !strings.Contains(stdout, "S3_BUCKET") {
		t.Errorf("stdout = %q, want both keys listed", stdout)
	}
}

func TestRun_StorageEnvKeysList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{"S3_REGION"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"storage-env-keys", "list", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"S3_REGION"`) {
		t.Errorf("stdout = %q, want the keys as a JSON array", stdout.String())
	}
}

func TestRun_StorageEnvKeysList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"storage-env-keys", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no storage env keys") {
		t.Errorf("stdout = %q, want a no-keys message", stdout.String())
	}
}

func TestRun_StorageEnvKeys_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"storage-env-keys", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
