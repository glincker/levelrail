package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setSSHPasswordForTest(t *testing.T, password string, err error) {
	t.Helper()
	orig := readSSHPassword
	readSSHPassword = func(io.Writer) (string, error) { return password, err }
	t.Cleanup(func() { readSSHPassword = orig })
}

func TestRun_NodesSSHProvision_KeyAuth(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("-----BEGIN PRIVATE KEY-----\nfakekeycontent\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	var gotMethod, gotPath string
	var gotBody createSSHNodeProvisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sshNodeProvisionResource{ID: "sshp_1", Name: gotBody.Name, Status: "connecting"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"nodes", "ssh-provision",
		"--host", "192.0.2.10", "--user", "root", "--key-file", keyFile,
		"--name", "web-1", "--control-plane-addr", "cp.example.com:9443",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/ssh-provision" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/ssh-provision", gotMethod, gotPath)
	}
	if gotBody.Host != "192.0.2.10" || gotBody.Username != "root" || gotBody.Name != "web-1" {
		t.Errorf("request body = %+v", gotBody)
	}
	if gotBody.Auth.Type != "key" || !strings.Contains(gotBody.Auth.PrivateKey, "fakekeycontent") {
		t.Errorf("auth = %+v, want the key file's content", gotBody.Auth)
	}
	if gotBody.Role != "general" {
		t.Errorf("Role = %q, want the default general", gotBody.Role)
	}
	if !strings.Contains(stdout, "sshp_1") || !strings.Contains(stdout, "connecting") {
		t.Errorf("stdout = %q, want the provision id and status", stdout)
	}
}

func TestRun_NodesSSHProvision_PasswordAuth(t *testing.T) {
	setSSHPasswordForTest(t, "s3cret", nil)

	var gotBody createSSHNodeProvisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sshNodeProvisionResource{ID: "sshp_2", Name: gotBody.Name, Status: "connecting"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "ssh-provision",
		"--host", "192.0.2.11", "--user", "root", "--password",
		"--name", "web-2", "--control-plane-addr", "cp.example.com:9443",
		"--api-url", srv.URL,
	})
	if gotBody.Auth.Type != "password" || gotBody.Auth.Password != "s3cret" {
		t.Errorf("auth = %+v", gotBody.Auth)
	}
}

func TestRun_NodesSSHProvision_MissingFlags(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	_ = os.WriteFile(keyFile, []byte("key"), 0o600)

	cases := [][]string{
		{"nodes", "ssh-provision", "--user", "root", "--key-file", keyFile, "--name", "n", "--control-plane-addr", "cp:9443"},
		{"nodes", "ssh-provision", "--host", "h", "--key-file", keyFile, "--name", "n", "--control-plane-addr", "cp:9443"},
		{"nodes", "ssh-provision", "--host", "h", "--user", "root", "--key-file", keyFile, "--control-plane-addr", "cp:9443"},
		{"nodes", "ssh-provision", "--host", "h", "--user", "root", "--name", "n", "--control-plane-addr", "cp:9443"},
		{"nodes", "ssh-provision", "--host", "h", "--user", "root", "--key-file", keyFile, "--password", "--name", "n", "--control-plane-addr", "cp:9443"},
	}
	for _, args := range cases {
		runCLIExpectValidationError(t, args)
	}
}

func TestRun_NodesSSHProvision_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "ssh-provision", "-h"})
	if !strings.Contains(stderr, "nodes ssh-provision") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_NodesSSHProvisions_ListAndShow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/ssh-node-provisions":
			_ = json.NewEncoder(w).Encode([]sshNodeProvisionResource{{ID: "sshp_1", Name: "web-1", Status: "ready"}})
		case "/api/v1/ssh-node-provisions/sshp_1":
			_ = json.NewEncoder(w).Encode(sshNodeProvisionResource{ID: "sshp_1", Name: "web-1", Status: "ready", Log: "connect: connected\ndone: agent active"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "ssh-provisions", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "sshp_1") || !strings.Contains(stdout, "web-1") {
		t.Errorf("list stdout = %q", stdout)
	}

	stdout, _ = runCLIExpectOK(t, []string{"nodes", "ssh-provisions", "show", "sshp_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "sshp_1") || !strings.Contains(stdout, "agent active") {
		t.Errorf("show stdout = %q", stdout)
	}
}
