package main

import (
	"bytes"
	"encoding/json"
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

func TestPointerSuppliesAddressWhenNoSavedLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	other, _, _ := countingServer(t)
	writePointer(t, other.URL)
	if got := resolveAPIURL("", envMap(), "lr-test-pointer-free", defaultProfile); got != other.URL {
		t.Errorf("url = %q, want the pointer address %q", got, other.URL)
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
