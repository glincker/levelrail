package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesCredentials(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseCredentialsResource{ //nolint:gosec // fake fixture, not a real credential
			Host: "db-main", Port: 5432, Database: "main", Username: "main",
			Password: "s3cr3t",
			URL:      "postgres://main:s3cr3t@db-main:5432/main",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "credentials", "main", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/databases/main/credentials" {
		t.Errorf("request = %s %s, want GET /api/v1/databases/main/credentials", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "host:     db-main") {
		t.Errorf("stdout = %q, want the host printed", stdout)
	}
	if !strings.Contains(stdout, "password: s3cr3t") {
		t.Errorf("stdout = %q, want the password printed", stdout)
	}
	if !strings.Contains(stdout, "url:      postgres://main:s3cr3t@db-main:5432/main") {
		t.Errorf("stdout = %q, want the url printed", stdout)
	}
}

func TestRun_DatabasesCredentials_RedisOmitsPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseCredentialsResource{
			Host: "db-cache", Port: 6379, URL: "redis://db-cache:6379",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "credentials", "cache", "--api-url", srv.URL})
	if strings.Contains(stdout, "password:") {
		t.Errorf("stdout = %q, want no password line for a passwordless engine", stdout)
	}
	if !strings.Contains(stdout, "url:      redis://db-cache:6379") {
		t.Errorf("stdout = %q, want the url printed", stdout)
	}
}

func TestRun_DatabasesCredentials_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "credentials", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a non-zero exit on a server error", got)
	}
	if !strings.Contains(stderr.String(), "get credentials for database") {
		t.Errorf("stderr = %q, want a get-credentials-failure message", stderr.String())
	}
}

func TestRun_DatabasesCredentials_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "credentials"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}
