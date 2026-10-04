package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AuthSessionLink(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sessionLinkResource{ //nolint:gosec // test fixture value, not a real credential
			Token: "ci-session-link-token",
			URL:   "https://dashboard.example.com/login?session_link=ci-session-link-token",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"auth", "session-link", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/session-links" {
		t.Errorf("request = %s %s, want POST /api/v1/auth/session-links", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "https://dashboard.example.com/login?session_link=ci-session-link-token") {
		t.Errorf("stdout = %q, want the minted URL", stdout)
	}
}

func TestRun_AuthSessionLink_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sessionLinkResource{Token: "sl_x", URL: "/login?session_link=sl_x"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"auth", "session-link", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"token": "sl_x"`) {
		t.Errorf("stdout = %q, want the token as JSON", stdout)
	}
}

func TestRun_AuthSessionLink_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusForbidden, `{"error":"token lacks the required ability"}`)

	stderr := runCLIExpectAPIError(t, []string{"auth", "session-link", "--api-url", srv.URL})
	if !strings.Contains(stderr, "token lacks the required ability") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AuthSessionLink_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"auth", "session-link", "-h"})
	if !strings.Contains(stderr, "auth session-link") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
