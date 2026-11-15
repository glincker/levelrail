package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// countingServer counts every request it receives and the ones carrying a bearer token.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var all, authed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		all.Add(1)
		if r.Header.Get("Authorization") != "" {
			authed.Add(1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &all, &authed
}

func writePointer(t *testing.T, apiURL string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".levelrail"), 0o750); err != nil {
		t.Fatal(err)
	}
	body := apiclient.RepoPointerContent("default", apiURL)
	if err := os.WriteFile(filepath.Join(dir, apiclient.RepoPointerPath), []byte(body), 0o644); err != nil { //nolint:gosec // test fixture, no secret
		t.Fatal(err)
	}
	t.Chdir(dir)
}

func TestPointerAddressDifferingFromSavedIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saved := newReuseServer(t, "live-token")
	other, all, _ := countingServer(t)
	const prog = "lr-test-pointer"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: saved.srv.URL, Token: "live-token", InstanceID: "inst-1", Fingerprint: "fp-1"}); err != nil {
		t.Fatal(err)
	}
	writePointer(t, other.URL)

	var out, errb bytes.Buffer
	if code := run(prog, []string{"whoami", "--json"}, &out, &errb, envMap()); code == exitOK {
		t.Fatalf("whoami succeeded, out %q", out.String())
	}
	var st sessionStatus
	_ = json.Unmarshal(out.Bytes(), &st)
	if st.Verified || !strings.Contains(st.Problem, "differs from the address saved") || !strings.Contains(st.Problem, saved.srv.URL) {
		t.Errorf("status = %+v, want the mismatch named", st)
	}

	client := apiClientFromFlags(prog, "", "", "", envMap())
	if _, err := client.Whoami(t.Context()); err == nil || !strings.Contains(err.Error(), "differs from the address saved") {
		t.Errorf("client error = %v, want the mismatch", err)
	}
	if code, _, e := runLoginCmd(t, prog); code == exitOK || !strings.Contains(e, "differs from the address saved") {
		t.Errorf("login exit %d err %q, want a refusal", code, e)
	}
	if all.Load() != 0 {
		t.Fatalf("pointer server received %d requests, want 0", all.Load())
	}
}

func TestPointerOnlyAddressIsNeverResolved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	other, _, _ := countingServer(t)
	writePointer(t, other.URL)
	if got := resolveAPIURL("", envMap(), "lr-test-pointer-free", defaultProfile); got != defaultAPIURL {
		t.Errorf("url = %q, want the default, not the pointer-only address", got)
	}
}

func TestEnvTokenNotSentToPointerOnlyAddress(t *testing.T) {
	cases := []struct {
		name, savedURL, want string
		env                  map[string]string
	}{
		{"app token, saved address differs", "http://127.0.0.1:1", "differs from the address saved", map[string]string{envAPIToken: "env-token"}},
		{"app token, nothing saved", "", "not saved in any profile", map[string]string{envAPIToken: "env-token"}},
		{"ci mode token", "", "not saved in any profile", map[string]string{envLevelrailToken: "ci-token", "CI": "1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			other, all, _ := countingServer(t)
			const prog = "lr-test-pointer-env"
			if c.savedURL != "" {
				if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: c.savedURL}); err != nil {
					t.Fatal(err)
				}
			}
			writePointer(t, other.URL)

			var out, errb bytes.Buffer
			if code := run(prog, []string{"whoami", "--json"}, &out, &errb, envOf(c.env)); code == exitOK {
				t.Fatalf("whoami succeeded, out %q", out.String())
			}
			var st sessionStatus
			_ = json.Unmarshal(out.Bytes(), &st)
			if !strings.Contains(st.Problem, c.want) || !strings.Contains(st.NextCommand, "--api-url "+other.URL) {
				t.Errorf("status = %+v, want %q and the deliberate command", st, c.want)
			}
			client := apiClientFromFlags(prog, "", "", "", envAliasLookup(envOf(c.env)))
			if _, err := client.Whoami(t.Context()); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("client error = %v, want %q", err, c.want)
			}
			if all.Load() != 0 {
				t.Fatalf("pointer server received %d requests, want 0", all.Load())
			}
		})
	}
}

func TestPointerMatchingSavedAddressIsUsed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rs := newReuseServer(t, "match-token")
	const prog = "lr-test-pointer-match"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: rs.srv.URL, InstanceID: "inst-1", Fingerprint: "fp-1"}); err != nil {
		t.Fatal(err)
	}
	writePointer(t, rs.srv.URL)
	env := envOf(map[string]string{envAPIToken: "match-token"})
	if _, err := apiClientFromFlags(prog, "", "", "", env).Whoami(t.Context()); err != nil {
		t.Fatalf("whoami through a pointer matching the saved address: %v", err)
	}

	rs2 := newReuseServer(t, "match-token-2")
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: rs2.srv.URL, InstanceID: "inst-other", Fingerprint: "fp-other"}); err != nil {
		t.Fatal(err)
	}
	writePointer(t, rs2.srv.URL)
	env = envOf(map[string]string{envAPIToken: "match-token-2"})
	var mismatch *apiclient.ServerMismatchError
	if _, err := apiClientFromFlags(prog, "", "", "", env).Whoami(t.Context()); !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want the server pin mismatch", err)
	}
}

func TestPointerPlainHTTPToAnotherMachineIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writePointer(t, "http://203.0.113.7:8080")
	env := envOf(map[string]string{envAPIToken: "env-token"})
	if _, err := apiClientFromFlags("lr-test-pointer-http", "", "", "", env).Whoami(t.Context()); err == nil || !strings.Contains(err.Error(), "plain http") {
		t.Errorf("client error = %v, want the plain http refusal", err)
	}
}

func TestFirstRunLoginRefusesPointerOnlyAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	other, all, _ := countingServer(t)
	const prog = "lr-test-pointer-first-run"
	writePointer(t, other.URL)
	prev := firstRunInteractive
	firstRunInteractive = func() bool { return false }
	t.Cleanup(func() { firstRunInteractive = prev })

	if _, err := apiClientFromFlags(prog, "", "", "", envMap()).Whoami(t.Context()); err == nil || !strings.Contains(err.Error(), "not saved in any profile") {
		t.Errorf("client error = %v, want the pointer refusal", err)
	}
	if code, _, e := runLoginCmd(t, prog); code == exitOK || !strings.Contains(e, "--api-url "+other.URL) {
		t.Errorf("login exit %d err %q, want a refusal naming --api-url", code, e)
	}
	m, _ := apiclient.FindPointerMismatch("", envMap(), prog, defaultProfile)
	var errb bytes.Buffer
	if _, _, ok := confirmPointerLogin(prog, defaultProfile, m, envMap(), strings.NewReader("\n"), &bytes.Buffer{}, &errb); ok {
		t.Error("sign-in went ahead without a typed yes")
	}
	if all.Load() != 0 {
		t.Fatalf("pointer server received %d requests, want 0", all.Load())
	}
}

func TestExplicitAddressBeatsPointer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rs := newReuseServer(t, "flag-token")
	other, all, _ := countingServer(t)
	writePointer(t, other.URL)
	const prog = "lr-test-pointer-explicit"
	env := envOf(map[string]string{envAPIToken: "flag-token"})
	if _, err := apiClientFromFlags(prog, rs.srv.URL, "", "", env).Whoami(t.Context()); err != nil {
		t.Fatalf("--api-url: %v", err)
	}
	env = envOf(map[string]string{envAPIToken: "flag-token", envAPIURL: rs.srv.URL})
	if _, err := apiClientFromFlags(prog, "", "", "", env).Whoami(t.Context()); err != nil {
		t.Fatalf("%s: %v", envAPIURL, err)
	}
	if all.Load() != 0 {
		t.Fatalf("pointer server received %d requests, want 0", all.Load())
	}
}

func TestLogin_UnpinnedProfileDoesNotSendSavedTokenToFlagURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	other, all, authed := countingServer(t)
	const prog = "lr-test-unpinned"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://127.0.0.1:1", Token: "saved-token"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reusableLogin(prog, "", other.URL, envMap()); ok {
		t.Fatal("login reused")
	}
	if authed.Load() != 0 {
		t.Fatalf("%d of %d requests carried the saved token, want 0", authed.Load(), all.Load())
	}
}

func TestWriteCredentials_ClearsTrustedDeviceWhenInstanceChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prog = "lr-test-device-clear"
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://a", Token: "t1", InstanceID: "inst-a", TrustedDevice: "dev-a"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://b", Token: "t2", InstanceID: "inst-b"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := readCredentialsFile(prog, defaultProfile); c.TrustedDevice != "" {
		t.Errorf("trusted device = %q, want it cleared for the new server", c.TrustedDevice)
	}
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://b", Token: "t3", InstanceID: "inst-b", TrustedDevice: "dev-b"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCredentialsFile(prog, defaultProfile, credentials{APIURL: "http://b", Token: "t4", InstanceID: "inst-b"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := readCredentialsFile(prog, defaultProfile); c.TrustedDevice != "dev-b" {
		t.Errorf("trusted device = %q, want it kept for the same server", c.TrustedDevice)
	}
}

func TestSavedDeviceOnlySeededForMatchingServer(t *testing.T) {
	rs := newReuseServer(t, "live-token")
	match := credentials{TrustedDevice: "dev-1", InstanceID: "inst-1", Fingerprint: "fp-1"}
	if !savedDeviceMatchesServer(match, rs.srv.URL) {
		t.Error("matching server should seed")
	}
	if savedDeviceMatchesServer(credentials{TrustedDevice: "dev-1", InstanceID: "inst-x", Fingerprint: "fp-1"}, rs.srv.URL) {
		t.Error("different instance must not seed")
	}
	if savedDeviceMatchesServer(credentials{TrustedDevice: "dev-1"}, rs.srv.URL) {
		t.Error("unpinned profile must not seed")
	}
}
