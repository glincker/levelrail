package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_NodesJoinToken(t *testing.T) {
	var gotMethod, gotPath string
	expires := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createNodeJoinTokenResponse{Token: "ci-join-token-value", ExpiresAt: expires}) //nolint:gosec // test fixture value, not a real credential
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "join-token", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/join-tokens" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/join-tokens", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "shown once, not recoverable again") {
		t.Errorf("stdout = %q, want the one-time-secret warning", stdout)
	}
	if !strings.Contains(stdout, "ci-join-token-value") {
		t.Errorf("stdout = %q, want the plaintext token", stdout)
	}
}

func TestRun_NodesJoinToken_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createNodeJoinTokenResponse{Token: "njtok_x", ExpiresAt: time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "join-token", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"token": "njtok_x"`) {
		t.Errorf("stdout = %q, want the token as JSON", stdout)
	}
}

func TestRun_NodesJoinToken_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "join-token", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesJoinToken_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "join-token", "-h"})
	if !strings.Contains(stderr, "nodes join-token") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
