package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func identityTestServer(id, finger string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/instance" {
			_, _ = w.Write([]byte(`{"instance_id":"` + id + `","name":"Test","fingerprint":"` + finger + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
}

func TestCheckLoginServer_RefusesPinnedProfileWithoutReauth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := identityTestServer("inst_BBBBBBBBBBBBBBBB", "cccc-dddd")
	defer srv.Close()
	pinned := credentials{APIURL: srv.URL, Token: "t", InstanceID: "inst_AAAAAAAAAAAAAAAA", Fingerprint: "aaaa-bbbb", ServerName: "Alpha"}
	if err := writeCredentialsFile("cli", "default", pinned); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if _, err := checkLoginServer("cli", "default", srv.URL, false, &stderr); err == nil || !strings.Contains(err.Error(), "--reauth") {
		t.Fatalf("err = %v, want refusal naming --reauth", err)
	}
	info, err := checkLoginServer("cli", "default", srv.URL, true, &stderr)
	if err != nil || info == nil || !strings.Contains(stderr.String(), "cccc-dddd") {
		t.Fatalf("with --reauth: info=%v err=%v stderr=%q, want pass and fingerprint printed", info, err, stderr.String())
	}
}

func TestConfirmTrust_PinsOnYesAndRefusesNonInteractive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := writeCredentialsFile("cli", "default", credentials{APIURL: "http://x", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	req := apiclient.TrustRequest{Prog: "cli", Profile: "default", URL: "http://x", Info: apiclient.InstanceInfo{InstanceID: "inst_AAAAAAAAAAAAAAAA", Name: "Alpha", Fingerprint: "aaaa-bbbb"}}
	var stderr bytes.Buffer

	if err := confirmTrust("cli", req, false, false, strings.NewReader(""), &stderr); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("non-interactive without --yes: err = %v, want refusal", err)
	}
	if got, _ := readCredentialsFile("cli", "default"); got.InstanceID != "" {
		t.Fatal("nothing may be pinned without confirmation")
	}
	if err := confirmTrust("cli", req, true, false, strings.NewReader(""), &stderr); err != nil {
		t.Fatalf("--yes: %v", err)
	}
	if got, _ := readCredentialsFile("cli", "default"); got.InstanceID != "inst_AAAAAAAAAAAAAAAA" || got.Token != "t" {
		t.Fatalf("pinned creds = %+v", got)
	}
	if err := confirmTrust("cli", req, false, true, strings.NewReader("n\n"), &stderr); err == nil {
		t.Fatal("answering n must decline")
	}
}
