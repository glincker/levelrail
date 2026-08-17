package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AuthWhoami_Success exercises the happy path against a fake
// server that (unlike the real control plane today, see
// auth_whoami.go's own doc comment on that gap) accepts a bearer token
// on GET /api/v1/auth/session, confirming this command's own request
// shape and output formatting are correct independent of that
// real-server limitation.
func TestRun_AuthWhoami_Success(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/auth/session" {
			t.Errorf("request = %s %s, want GET /api/v1/auth/session", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionInfoResource{Username: "admin", ExpiresAt: "2026-08-16T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "whoami", "--token", "sometoken", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotAuth != "Bearer sometoken" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer sometoken")
	}
	if !strings.Contains(stdout.String(), "admin") {
		t.Errorf("stdout = %q, want it to show the username", stdout.String())
	}
}

// TestRun_AuthWhoami_RealServer401 is the actual, documented behavior
// against internal/api's real handleGetSession: session-cookie-only, so
// a bearer-token-only caller (this CLI's only supported auth mechanism)
// gets a real 401, and this command must report that honestly rather
// than pretending success.
func TestRun_AuthWhoami_RealServer401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session_token"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionInfoResource{Username: "admin"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "whoami", "--token", "sometoken", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "authentication required") {
		t.Errorf("stderr = %q, want the server's real error message", stderr.String())
	}
}

func TestRun_AuthWhoami_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "whoami", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "auth whoami") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestRun_Auth_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown auth subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
