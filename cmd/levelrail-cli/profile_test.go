package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_ProfileList_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout, _ := runCLIExpectOK(t, []string{"profile", "list"})
	if !strings.Contains(stdout, "no profiles configured") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

// TestRun_ProfileList_ShowsConfiguredProfiles confirms every configured
// profile's name and API URL show up, and that the token value never
// does, matching this CLI's standing "never print a secret" rule.
func TestRun_ProfileList_ShowsConfiguredProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prog := "levelrail-cli-test"

	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://home:8080", Token: "home-secret-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(default) error = %v", err)
	}
	if err := writeCredentialsFile(prog, "work", credentials{APIURL: "https://work.example.com", Token: "work-secret-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(work) error = %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"profile", "list"})
	if !strings.Contains(stdout, "default") || !strings.Contains(stdout, "http://home:8080") {
		t.Errorf("stdout = %q, want the default profile listed", stdout)
	}
	if !strings.Contains(stdout, "work") || !strings.Contains(stdout, "https://work.example.com") {
		t.Errorf("stdout = %q, want the work profile listed", stdout)
	}
	if strings.Contains(stdout, "home-secret-token") || strings.Contains(stdout, "work-secret-token") {
		t.Errorf("stdout = %q, must never contain a token value", stdout)
	}
}

func TestRun_ProfileList_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prog := "levelrail-cli-test"

	if err := writeCredentialsFile(prog, "work", credentials{APIURL: "https://work.example.com", Token: "work-secret-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(work) error = %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"profile", "list", "--json"})
	var got []profileSummary
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout, err)
	}
	if len(got) != 1 || got[0].Name != "work" || got[0].APIURL != "https://work.example.com" {
		t.Errorf("got = %+v, want a single work profile", got)
	}
	if strings.Contains(stdout, "work-secret-token") {
		t.Errorf("stdout = %q, must never contain a token value even as JSON", stdout)
	}
}

// authRecordingServer starts a test server recording the Authorization
// header of every request it receives, so a test can assert which
// profile's token a command actually used.
func authRecordingServer(t *testing.T) (srv *httptest.Server, gotAuth *string) {
	t.Helper()
	gotAuth = new(string)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]nodeResource{})
	}))
	t.Cleanup(srv.Close)
	return srv, gotAuth
}

// TestProfileFlag_SelectsNamedProfile confirms --profile NAME resolves
// that profile's own token/API URL, not "default"'s, and that omitting
// --profile still falls back to "default": the flag > env > default
// precedence chain end to end, not just at the apiclient unit level.
func TestProfileFlag_SelectsNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prog := "levelrail-cli-test"

	defaultSrv, defaultAuth := authRecordingServer(t)
	workSrv, workAuth := authRecordingServer(t)

	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: defaultSrv.URL, Token: "default-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(default) error = %v", err)
	}
	if err := writeCredentialsFile(prog, "work", credentials{APIURL: workSrv.URL, Token: "work-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(work) error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if got := run(prog, []string{"nodes", "list", "--profile", "work"}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if *workAuth != "Bearer work-token" {
		t.Errorf("work server auth = %q, want Bearer work-token", *workAuth)
	}
	if *defaultAuth != "" {
		t.Errorf("default server auth = %q, want no request reached it", *defaultAuth)
	}

	stdout.Reset()
	stderr.Reset()
	*workAuth, *defaultAuth = "", ""
	if got := run(prog, []string{"nodes", "list"}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if *defaultAuth != "Bearer default-token" {
		t.Errorf("default server auth = %q, want Bearer default-token (no --profile given)", *defaultAuth)
	}
}

// TestAPPProfileEnvVar_SelectsNamedProfile confirms the APP_PROFILE env
// var selects a profile the same way --profile does, and that an
// explicit --profile flag still wins over it.
func TestAPPProfileEnvVar_SelectsNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prog := "levelrail-cli-test"

	workSrv, workAuth := authRecordingServer(t)
	otherSrv, otherAuth := authRecordingServer(t)

	if err := writeCredentialsFile(prog, "work", credentials{APIURL: workSrv.URL, Token: "work-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(work) error = %v", err)
	}
	if err := writeCredentialsFile(prog, "other", credentials{APIURL: otherSrv.URL, Token: "other-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(other) error = %v", err)
	}

	envWithProfile := func(key string) (string, bool) {
		if key == envProfile {
			return "work", true
		}
		return "", false
	}

	var stdout, stderr bytes.Buffer
	if got := run(prog, []string{"nodes", "list"}, &stdout, &stderr, envWithProfile); got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if *workAuth != "Bearer work-token" {
		t.Errorf("work server auth = %q, want Bearer work-token (via APP_PROFILE)", *workAuth)
	}

	// An explicit --profile flag still wins over APP_PROFILE.
	stdout.Reset()
	stderr.Reset()
	*workAuth, *otherAuth = "", ""
	if got := run(prog, []string{"nodes", "list", "--profile", "other"}, &stdout, &stderr, envWithProfile); got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if *otherAuth != "Bearer other-token" {
		t.Errorf("other server auth = %q, want Bearer other-token (--profile beats APP_PROFILE)", *otherAuth)
	}
}

// TestTokenFlag_OverridesProfile confirms an explicit --token still wins
// over a resolved profile's own token, matching AWS CLI's own "an
// explicit flag always wins" precedence.
func TestTokenFlag_OverridesProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prog := "levelrail-cli-test"

	srv, gotAuth := authRecordingServer(t)

	if err := writeCredentialsFile(prog, "work", credentials{APIURL: srv.URL, Token: "work-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(work) error = %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "list", "--profile", "work", "--token", "explicit-flag-token"})
	_ = stdout
	if *gotAuth != "Bearer explicit-flag-token" {
		t.Errorf("server auth = %q, want the explicit --token flag to win over the work profile's own token", *gotAuth)
	}
}

// TestRun_AuthLogin_NamedProfile_DoesNotClobberDefault confirms "auth
// login --profile NAME" saves under that named section, leaving any
// existing "default" section (or any other named profile) untouched.
func TestRun_AuthLogin_NamedProfile_DoesNotClobberDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "levelrail-cli-test-auth-login-profile"

	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://existing:8080", Token: "existing-default-token"}); err != nil {
		t.Fatalf("writeCredentialsFile(default) error = %v", err)
	}

	srv := fakeControlPlane(t, []string{"root"})

	var stdout, stderr bytes.Buffer
	got := run(prog, []string{"auth", "login", "--username", "admin", "--password", "correct-horse", "--api-url", srv.URL, "--profile", "work"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `under profile "work"`) {
		t.Errorf("stdout = %q, want it to mention the work profile", stdout.String())
	}

	workCreds, err := readCredentialsFile(prog, "work")
	if err != nil {
		t.Fatalf("readCredentialsFile(work) error = %v", err)
	}
	if workCreds.Token != "plaintext-token-value" || workCreds.APIURL != srv.URL {
		t.Errorf("work profile = %+v, want the freshly minted token and %s", workCreds, srv.URL)
	}

	defaultCreds, err := readCredentialsFile(prog, defaultProfile)
	if err != nil {
		t.Fatalf("readCredentialsFile(default) error = %v", err)
	}
	if defaultCreds.Token != "existing-default-token" || defaultCreds.APIURL != "http://existing:8080" {
		t.Errorf("default profile = %+v, want it unchanged by a --profile work login", defaultCreds)
	}
}
