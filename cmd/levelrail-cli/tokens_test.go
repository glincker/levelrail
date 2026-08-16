package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSessionAuthServer wires up the login cookie plus one more
// session-only route ("route"/"method" gate everything else), the same
// session-establishing shape fakeControlPlane (auth_login_test.go) uses,
// reused here for tokens list/revoke instead of tokens create.
func fakeSessionAuthServer(t *testing.T, method, path string, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	const cookieName = "session_token"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "sess-xyz", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // deliberately not Secure: this fake models the login-then-session-only-route happy path over httptest.NewServer's own plain-http listener, the same reasoning auth_login_test.go's own fakeControlPlane documents
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{Username: req.Username})
	})
	mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(cookieName); err != nil || c.Value != "sess-xyz" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_TokensCreate(t *testing.T) {
	var gotReq createTokenRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode create token request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createTokenResponse{
			tokenResource: tokenResource{ID: "tok_ci", Name: gotReq.Name, Abilities: gotReq.Abilities, CreatedAt: time.Now()},
			Token:         "ci-token-value",
		})
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"tokens", "create", "--name", "ci", "--abilities", "read,deploy",
		"--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotReq.Name != "ci" || len(gotReq.Abilities) != 2 {
		t.Errorf("request = %+v, want name=ci abilities=[read deploy]", gotReq)
	}
	if !strings.Contains(stdout.String(), "ci-token-value") {
		t.Errorf("stdout = %q, want it to show the minted token once", stdout.String())
	}
}

func TestRun_TokensCreate_MissingAbilities(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "create", "--name", "ci", "--username", "admin", "--password", "x"}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--abilities is required") {
		t.Errorf("stderr = %q, want a missing-abilities validation error", stderr.String())
	}
}

func TestRun_TokensList(t *testing.T) {
	srv := fakeSessionAuthServer(t, "GET", "/api/v1/auth/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tokenResource{
			{ID: "tok_1", Name: "ci", Abilities: []string{"read"}, CreatedAt: time.Now()},
		})
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "list", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "tok_1") {
		t.Errorf("stdout = %q, want the token id listed", stdout.String())
	}
	if strings.Contains(stdout.String(), "ci-token-value") {
		t.Errorf("stdout = %q, must never contain a token secret", stdout.String())
	}
}

func TestRun_TokensRevoke(t *testing.T) {
	var gotPath string
	srv := fakeSessionAuthServer(t, "DELETE", "/api/v1/auth/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "revoke", "tok_1", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/auth/tokens/tok_1" {
		t.Errorf("path = %q, want /api/v1/auth/tokens/tok_1", gotPath)
	}
	if !strings.Contains(stdout.String(), "revoked") {
		t.Errorf("stdout = %q, want a revoked confirmation", stdout.String())
	}
}

func TestRun_TokensRevoke_NoID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "revoke", "--username", "admin", "--password", "x"}, &stdout, &stderr, envMap(nil))
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_Tokens_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "-h"}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "tokens create") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}
