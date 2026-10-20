package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var testCLIHostKeyFP = "SHA256:" + strings.Repeat("q", 43)

type sshProvisionTestServer struct {
	srv           *httptest.Server
	preflightHits int
	created       createSSHNodeProvisionRequest
	createHits    int
	warnings      []string
}

func newSSHProvisionTestServer(t *testing.T) *sshProvisionTestServer {
	t.Helper()
	ts := &sshProvisionTestServer{}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/nodes/ssh-provision/preflight":
			ts.preflightHits++
			_ = json.NewEncoder(w).Encode(sshProvisionPreflight{Host: "192.0.2.10", Port: 22, HostKeys: []apiclient.SSHHostKey{
				{Type: "ssh-ed25519", Fingerprint: testCLIHostKeyFP},
				{Type: "ssh-rsa", Fingerprint: "SHA256:" + strings.Repeat("r", 43)},
			}})
		case "/api/v1/nodes/ssh-provision":
			ts.createHits++
			_ = json.NewDecoder(r.Body).Decode(&ts.created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(sshNodeProvisionResource{ID: "sshp_9", Name: ts.created.Name, Status: "connecting", Warnings: ts.warnings})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func setConfirmSSHHostKeyForTest(t *testing.T, err error) *string {
	t.Helper()
	orig := confirmSSHHostKey
	shown := new(string)
	confirmSSHHostKey = func(_ io.Writer, key string) error { *shown = key; return err }
	t.Cleanup(func() { confirmSSHHostKey = orig })
	return shown
}

func sshProvisionArgs(srvURL string, extra ...string) []string {
	args := []string{
		"nodes", "ssh-provision", "--host", "192.0.2.10", "--user", "root", "--password",
		"--name", "web-9", "--control-plane-addr", "cp.example.com:9443", "--api-url", srvURL,
	}
	return append(args, extra...)
}

func TestRun_NodesSSHProvision_HostKeyFlows(t *testing.T) {
	tests := []struct {
		name          string
		extra         []string
		confirmErr    error
		wantOK        bool
		wantPreflight bool
		wantFP        string
		wantInsecure  bool
		wantStderr    string
	}{
		{name: "interactive confirm pins first key", wantOK: true, wantPreflight: true, wantFP: testCLIHostKeyFP, wantStderr: "ssh-keygen -lf"},
		{name: "interactive decline sends nothing", confirmErr: errors.New("cancelled: host key not confirmed"), wantPreflight: true},
		{name: "flag pins without preflight", extra: []string{"--host-key-fingerprint", testCLIHostKeyFP}, wantOK: true, wantFP: testCLIHostKeyFP},
		{name: "insecure flag warns", extra: []string{"--insecure-skip-host-key-check"}, wantOK: true, wantInsecure: true, wantStderr: "WARNING"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setSSHPasswordForTest(t, "s3cret", nil)
			shown := setConfirmSSHHostKeyForTest(t, tc.confirmErr)
			ts := newSSHProvisionTestServer(t)
			var stdout, stderr strings.Builder
			code := run("levelrail-cli-test", sshProvisionArgs(ts.srv.URL, tc.extra...), &stdout, &stderr, envMap())
			if (code == exitOK) != tc.wantOK {
				t.Fatalf("exit = %d, wantOK %v, stderr = %q", code, tc.wantOK, stderr.String())
			}
			if (ts.preflightHits > 0) != tc.wantPreflight {
				t.Fatalf("preflight hits = %d, want preflight %v", ts.preflightHits, tc.wantPreflight)
			}
			if tc.wantPreflight && !strings.Contains(*shown, testCLIHostKeyFP) {
				t.Fatalf("confirmation showed %q, want the ed25519 fingerprint", *shown)
			}
			if !tc.wantOK {
				if ts.createHits != 0 {
					t.Fatalf("credentials were sent after the operator declined")
				}
				return
			}
			if ts.created.HostKeyFingerprint != tc.wantFP || ts.created.InsecureSkipHostKeyCheck != tc.wantInsecure {
				t.Fatalf("create body host key = %q/%v", ts.created.HostKeyFingerprint, ts.created.InsecureSkipHostKeyCheck)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

func TestRun_NodesSSHProvision_HostKeyFlagsExclusive(t *testing.T) {
	ts := newSSHProvisionTestServer(t)
	runCLIExpectValidationError(t, sshProvisionArgs(ts.srv.URL, "--host-key-fingerprint", testCLIHostKeyFP, "--insecure-skip-host-key-check"))
	if ts.preflightHits+ts.createHits != 0 {
		t.Fatalf("server was called despite conflicting flags")
	}
}

func TestRun_NodesSSHProvision_ServerWarningsPrinted(t *testing.T) {
	setSSHPasswordForTest(t, "s3cret", nil)
	ts := newSSHProvisionTestServer(t)
	ts.warnings = []string{"host key verification was skipped"}
	_, stderr := runCLIExpectOK(t, sshProvisionArgs(ts.srv.URL, "--insecure-skip-host-key-check"))
	if !strings.Contains(stderr, "host key verification was skipped") {
		t.Fatalf("stderr = %q, want the server warning", stderr)
	}
}
