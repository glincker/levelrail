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

// fakeControlPlane wires up a real httptest.Server implementing the
// two real endpoints "auth login" calls in sequence: POST
// /api/v1/auth/login (sets a session cookie) and POST
// /api/v1/auth/tokens (only honored when that cookie is present, the
// same session-only gate router.go's real registration enforces),
// mirroring auth.go's handleLogin and tokens.go's handleCreateToken
// closely enough to exercise this CLI's two-step flow honestly,
// including the part where a missing cookie on the second call is a
// real 401, not a client bug.
func fakeControlPlane(t *testing.T, wantAbilities []string) *httptest.Server {
	t.Helper()
	const cookieName = "session_token"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		if req.Username != "admin" || req.Password != "correct-horse" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "sess-abc", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // deliberately not Secure: this fake models the login-then-mint-token happy path over httptest.NewServer's own plain-http listener; the real server's cookie is Secure (auth.go's handleLogin), and TestRun_AuthWhoami_RealServer401/authclient.go's own doc comment cover the plain-http consequence of that separately
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{Username: req.Username})
	})
	mux.HandleFunc("POST /api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value != "sess-abc" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		var req createTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode create token request: %v", err)
		}
		if wantAbilities != nil {
			if len(req.Abilities) != len(wantAbilities) {
				t.Errorf("abilities = %v, want %v", req.Abilities, wantAbilities)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createTokenResponse{
			tokenResource: tokenResource{ID: "tok_1", Name: req.Name, Abilities: req.Abilities},
			Token:         "plaintext-token-value",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestRun_AuthLogin_MintsAndPersistsToken exercises "auth login" end to
// end: it must actually call POST /api/v1/auth/login, carry the
// resulting session cookie into POST /api/v1/auth/tokens (only a real
// cookie jar makes that happen, see authclient.go's own doc comment),
// and then write the returned token to the credentials file config.go's
// resolveToken/readCredentialsFile pair later reads.
func TestRun_AuthLogin_MintsAndPersistsToken(t *testing.T) {
	srv := fakeControlPlane(t, []string{"root"})

	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "levelrail-cli-test-auth-login"

	var stdout, stderr bytes.Buffer
	got := run(prog, []string{"auth", "login", "--username", "admin", "--password", "correct-horse", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "plaintext-token-value") {
		t.Errorf("stdout = %q, want it to show the minted token once", stdout.String())
	}

	credPath := filepath.Join(home, ".config", prog, credentialsFileName)
	data, err := os.ReadFile(credPath) //nolint:gosec // test-only path built from t.TempDir()+a fixed filename, not external input
	if err != nil {
		t.Fatalf("credentials file not written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "APP_API_TOKEN=plaintext-token-value") {
		t.Errorf("credentials file = %q, want it to contain the minted token", content)
	}
	if !strings.Contains(content, "APP_API_URL="+srv.URL) {
		t.Errorf("credentials file = %q, want it to contain the api url", content)
	}

	// And a later command resolves that persisted token automatically,
	// the entire point of "auth login" existing.
	got2 := resolveToken("", envMap(), prog)
	if got2 != "plaintext-token-value" {
		t.Errorf("resolveToken() after login = %q, want the persisted token", got2)
	}
}

func TestRun_AuthLogin_JSONWritesFullTokenResource(t *testing.T) {
	srv := fakeControlPlane(t, []string{"read"})

	home := t.TempDir()
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test-auth-login-json", []string{
		"auth", "login", "--username", "admin", "--password", "correct-horse",
		"--api-url", srv.URL, "--abilities", "read", "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"token": "plaintext-token-value"`) {
		t.Errorf("stdout = %q, want the full created-token JSON including the raw token", stdout.String())
	}
}

func TestRun_AuthLogin_WrongPassword(t *testing.T) {
	srv := fakeControlPlane(t, nil)

	home := t.TempDir()
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test-auth-login-badpw", []string{"auth", "login", "--username", "admin", "--password", "wrong", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid credentials") {
		t.Errorf("stderr = %q, want the server's real error message", stderr.String())
	}
}

func TestRun_AuthLogin_MissingUsername(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	// Calls runAuthLogin directly (not through run(), which always
	// wires the real os.Stdin) so an empty stdin.Reader is fully
	// controlled: promptUsername reads a line, gets EOF immediately
	// (no line ever typed), and this asserts that failure is reported
	// as a real error, never a hang or a silent empty-string login
	// attempt.
	got := runAuthLogin("levelrail-cli-test-auth-login-nouser", []string{"--password", "x"}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got == exitOK {
		t.Fatalf("exit = %d, want a failure (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "read username") {
		t.Errorf("stderr = %q, want it to report the username prompt failing", stderr.String())
	}
}

func TestRun_AuthLogin_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "login", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "auth login") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
