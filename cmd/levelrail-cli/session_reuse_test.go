package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// reuseServer models a control plane that issues a device cookie on login and accepts one live token.
type reuseServer struct {
	srv        *httptest.Server
	logins     atomic.Int32
	logouts    atomic.Int32
	deviceSeen atomic.Int32
}

func newReuseServer(t *testing.T, goodToken string) *reuseServer {
	t.Helper()
	rs := &reuseServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/instance", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.InstanceInfo{InstanceID: "inst-1", Name: "test", Fingerprint: "fp-1"})
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		rs.logins.Add(1)
		if c, err := r.Cookie("trusted_device"); err == nil && c.Value == "dev-1" {
			rs.deviceSeen.Add(1)
		}
		http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "sess", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})                 //nolint:gosec // plain-http fake
		http.SetCookie(w, &http.Cookie{Name: "trusted_device", Value: "dev-1", Path: "/api/v1/auth", HttpOnly: true, SameSite: http.SameSiteStrictMode}) //nolint:gosec // plain-http fake
		_ = json.NewEncoder(w).Encode(loginResponse{Username: "admin"})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		rs.logouts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/auth/tokens", func(w http.ResponseWriter, _ *http.Request) {
		exp := time.Now().Add(90 * 24 * time.Hour)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createTokenResponse{tokenResource: tokenResource{ID: "tok", Name: "n", ExpiresAt: &exp}, Token: goodToken})
	})
	mux.HandleFunc("GET /api/v1/auth/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+goodToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "token", "name": "admin", "abilities": []string{"root"}})
	})
	rs.srv = httptest.NewServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

func runLoginCmd(t *testing.T, prog string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	full := append([]string{"auth", "login", "--username", "admin", "--password", "pw"}, args...)
	code := run(prog, full, &out, &errb, envMap())
	return code, out.String(), errb.String()
}

func TestLogin_ReusesValidLoginAndKeepsDeviceID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rs := newReuseServer(t, "live-token")
	const prog = "lr-test-reuse"

	if code, _, e := runLoginCmd(t, prog, "--api-url", rs.srv.URL); code != exitOK {
		t.Fatalf("first login exit %d: %s", code, e)
	}
	creds, _ := readCredentialsFile(prog, defaultProfile)
	if creds.TrustedDevice != "dev-1" || creds.TokenExpiresAt == "" {
		t.Fatalf("saved creds = %+v, want the device id and expiry stored", creds)
	}
	if rs.logouts.Load() != 1 {
		t.Errorf("logouts = %d, want the login session ended once", rs.logouts.Load())
	}

	code, out, e := runLoginCmd(t, prog, "--api-url", rs.srv.URL)
	if code != exitOK || !strings.Contains(out, "already signed in") {
		t.Fatalf("second login exit %d out %q err %q, want a reuse", code, out, e)
	}
	if rs.logins.Load() != 1 {
		t.Fatalf("logins = %d, a valid login must not create another session", rs.logins.Load())
	}

	if code, _, e := runLoginCmd(t, prog, "--api-url", rs.srv.URL, "--force"); code != exitOK {
		t.Fatalf("forced login exit %d: %s", code, e)
	}
	if rs.deviceSeen.Load() != 1 {
		t.Errorf("server saw the saved device id %d times on the forced login, want 1", rs.deviceSeen.Load())
	}
	again, _ := readCredentialsFile(prog, defaultProfile)
	if again.TrustedDevice != "dev-1" {
		t.Errorf("device id changed to %q", again.TrustedDevice)
	}
}

func TestLogin_ExpiredCredentialFallsThroughToLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rs := newReuseServer(t, "fresh-token")
	const prog = "lr-test-expired"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: rs.srv.URL, Token: "stale-token", TrustedDevice: "dev-1", InstanceID: "inst-1", Fingerprint: "fp-1"}); err != nil {
		t.Fatal(err)
	}
	code, out, e := runLoginCmd(t, prog, "--api-url", rs.srv.URL)
	if code != exitOK || strings.Contains(out, "already signed in") {
		t.Fatalf("exit %d out %q err %q, want a fresh login", code, out, e)
	}
	if rs.logins.Load() != 1 || rs.deviceSeen.Load() != 1 {
		t.Errorf("logins=%d deviceSeen=%d, want one login that presented the saved device id", rs.logins.Load(), rs.deviceSeen.Load())
	}
	if c, _ := readCredentialsFile(prog, defaultProfile); c.Token != "fresh-token" {
		t.Errorf("token = %q, want the new one saved", c.Token)
	}
}

func TestWhoami_NoCredentialPrintsExactFix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errb bytes.Buffer
	code := run("lr-test-none", []string{"whoami", "--json", "--api-url", "http://localhost:1"}, &out, &errb, envMap())
	if code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	var st sessionStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", out.String(), err)
	}
	if st.HasCredential || st.Usable || !strings.Contains(st.NextCommand, "login --api-url http://localhost:1 --profile default") || st.RunBy == "" {
		t.Errorf("status = %+v, want the exact login command and who runs it", st)
	}
}

func TestWhoami_VerifiesLive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rs := newReuseServer(t, "live-token")
	const prog = "lr-test-who"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: rs.srv.URL, Token: "live-token", InstanceID: "inst-1", Fingerprint: "fp-1"}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(prog, []string{"whoami", "--json"}, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var st sessionStatus
	_ = json.Unmarshal(out.Bytes(), &st)
	if !st.Verified || st.Identity != "admin" || strings.Contains(out.String(), "live-token") {
		t.Errorf("status = %+v, out = %q", st, out.String())
	}
}

func TestLogin_NonTerminalFailsWithTheCommand(t *testing.T) {
	old := ciMode
	ciMode = true
	t.Cleanup(func() { ciMode = old })
	err := requireHumanForLogin("lr", "default", "http://h:1", sessionFlags{}, stdinSource)
	if err == nil {
		t.Skip("stdinSource is not os.Stdin here")
	}
	for _, want := range []string{"not a terminal", "lr login --api-url http://h:1 --profile default", "whoami --json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestNoteExpiry_WarnsOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prog = "lr-test-warn"
	soon := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://x", Token: "t", TokenExpiresAt: soon}); err != nil {
		t.Fatal(err)
	}
	expiryWarned = map[string]bool{}
	var b bytes.Buffer
	noteExpiry(prog, defaultProfile, "t", envMap(), &b)
	noteExpiry(prog, defaultProfile, "t", envMap(), &b)
	if n := strings.Count(b.String(), "expires in"); n != 1 || !strings.Contains(b.String(), "login --api-url http://x") {
		t.Errorf("warning output = %q, want exactly one warning naming the fix", b.String())
	}
}

func TestInit_WritesPointerWithoutSecrets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	var out, errb bytes.Buffer
	args := []string{"init", "--dir", dir, "--yes", "--profile", "work", "--api-url", "https://cp.example.com"}
	if code := run("lr-test-init", args, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	got, ok := apiclient.FindRepoPointer(dir)
	if !ok || got.Profile != "work" || got.APIURL != "https://cp.example.com" {
		t.Fatalf("pointer = %+v ok=%v", got, ok)
	}
}

func TestHumanLeft_DaysThenHours(t *testing.T) {
	cases := map[time.Duration]string{2160 * time.Hour: "90 days", 49 * time.Hour: "2 days", 47 * time.Hour: "47 hours", 2 * time.Hour: "2 hours", 10 * time.Minute: "under an hour", -time.Minute: "expired"}
	for d, want := range cases {
		if got := humanLeft(d); got != want {
			t.Errorf("humanLeft(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestExpiryOutput_UsesDaysNotHours(t *testing.T) {
	soon := time.Now().Add(2160 * time.Hour).UTC().Format(time.RFC3339)
	if got := expiryNote(soon); !strings.Contains(got, "in 89 days") && !strings.Contains(got, "in 90 days") || strings.Contains(got, "h0m") {
		t.Errorf("expiryNote = %q", got)
	}
	var b bytes.Buffer
	printProfileRows(&b, []profileRow{{Name: "p", ExpiresAt: soon}})
	if !strings.Contains(b.String(), "days") || strings.Contains(b.String(), "h0m") {
		t.Errorf("profile list = %q", b.String())
	}
}

func TestRejectedToken_ErrorEndsWithNextStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"token revoked"}`))
	}))
	defer srv.Close()
	t.Setenv("HOME", t.TempDir())
	c := apiClientFromFlags("lr-test-401", srv.URL, "stale", "", envMap())
	_, err := c.Whoami(context.Background())
	want := nextStepLine("lr-test-401", defaultProfile, srv.URL)
	if err == nil || !strings.Contains(err.Error(), "token revoked") || !strings.HasSuffix(err.Error(), want) || !strings.Contains(want, "whoami --json") {
		t.Errorf("err = %v, want suffix %q", err, want)
	}
}

func TestLoginForce_RevokesPreviousSavedToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prog = "lr-test-force"
	var revoked []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(loginResponse{Username: "admin"})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /api/v1/auth/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createTokenResponse{tokenResource: tokenResource{ID: "tok_new", Name: "new"}, Token: "new-tok"})
	})
	mux.HandleFunc("GET /api/v1/auth/tokens", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]tokenResource{{ID: "tok_old", Name: "old"}, {ID: "tok_new", Name: "new"}})
	})
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		revoked = append(revoked, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/auth/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "token", "name": "old"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: srv.URL, Token: "old-tok"}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	args := []string{"auth", "login", "--force", "--api-url", srv.URL, "--username", "admin", "--password", "pw"}
	if code := run(prog, args, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("exit = %d, stderr %q", code, errb.String())
	}
	if len(revoked) != 1 || revoked[0] != "tok_old" {
		t.Errorf("revoked = %v, want only tok_old", revoked)
	}

	// An env-sourced token is never revoked.
	revoked = nil
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: srv.URL, Token: "old-tok"}); err != nil {
		t.Fatal(err)
	}
	env := func(k string) (string, bool) {
		if k == envAPIToken {
			return "env-tok", true
		}
		return "", false
	}
	if _, ok := savedLoginToReplace(prog, defaultProfile, srv.URL, env); ok {
		t.Errorf("with an environment token set, nothing may be revoked")
	}
}
