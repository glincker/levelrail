package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newRegistryCredentialEchoServer starts a test server that decodes a
// create or update request into gotBody and responds with a registry
// credential resource built from the request plus id, recording the
// request path and method when the caller passes non-nil pointers.
func newRegistryCredentialEchoServer(t *testing.T, id string, gotPath, gotMethod *string) (*httptest.Server, *createRegistryCredentialRequest) {
	t.Helper()
	gotBody := &createRegistryCredentialRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if gotMethod != nil {
			*gotMethod = r.Method
		}
		if err := json.NewDecoder(r.Body).Decode(gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(registryCredentialResource{
			ID: id, Name: gotBody.Name, RegistryHost: gotBody.RegistryHost, Username: gotBody.Username,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, gotBody
}

func TestRun_RegistryCredentialsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []registryCredentialResource{
		{ID: "regcred_1", Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot", CreatedAt: "2026-08-17T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry-credentials", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/registry-credentials" {
		t.Errorf("path = %q, want /api/v1/registry-credentials", gotPath)
	}
	if !strings.Contains(stdout, "regcred_1") {
		t.Errorf("stdout = %q, want the credential id listed", stdout)
	}
}

func TestRun_RegistryCredentialsList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []registryCredentialResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry-credentials", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no registry credentials") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_RegistryCredentialsGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry-credentials/regcred_1" {
			t.Errorf("path = %q, want /api/v1/registry-credentials/regcred_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryCredentialResource{ID: "regcred_1", Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"registry-credentials", "get", "regcred_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "ghcr-bot") || !strings.Contains(stdout, "ghcr.io") {
		t.Errorf("stdout = %q, want name and registry host shown", stdout)
	}
}

func TestRun_RegistryCredentialsGet_MissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry-credentials", "get"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
	}
}

func TestRun_RegistryCredentialsCreate(t *testing.T) {
	var gotPath, gotMethod string
	srv, gotBody := newRegistryCredentialEchoServer(t, "regcred_2", &gotPath, &gotMethod)

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"registry-credentials", "create", "--name", "ghcr-bot", "--registry-host", "ghcr.io",
		"--username", "bot", "--password", "gh-token-real", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/registry-credentials" || gotMethod != http.MethodPost {
		t.Errorf("request = %s %s, want POST /api/v1/registry-credentials", gotMethod, gotPath)
	}
	if gotBody.Password != "gh-token-real" {
		t.Errorf("request body = %+v, want the given password", gotBody)
	}
	if strings.Contains(stdout.String(), "gh-token-real") {
		t.Errorf("stdout = %q, password must never be printed back", stdout.String())
	}
}

func TestRun_RegistryCredentialsCreate_MissingRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing name", args: []string{"--registry-host", "ghcr.io", "--username", "u", "--password", "p"}, want: "--name is required"},
		{name: "missing registry host", args: []string{"--name", "n", "--username", "u", "--password", "p"}, want: "--registry-host is required"},
		{name: "missing username", args: []string{"--name", "n", "--registry-host", "ghcr.io", "--password", "p"}, want: "--username is required"},
		{name: "missing password", args: []string{"--name", "n", "--registry-host", "ghcr.io", "--username", "u"}, want: "--password is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append([]string{"registry-credentials", "create"}, tt.args...), &stdout, &stderr, envMap())
			if got != exitValidation {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRun_RegistryCredentialsUpdate(t *testing.T) {
	var gotPath, gotMethod string
	srv, gotBody := newRegistryCredentialEchoServer(t, "regcred_1", &gotPath, &gotMethod)

	stdout, _ := runCLIExpectOK(t, []string{
		"registry-credentials", "update", "regcred_1", "--name", "renamed", "--registry-host", "ghcr.io",
		"--username", "new-user", "--api-url", srv.URL,
	})
	if gotPath != "/api/v1/registry-credentials/regcred_1" || gotMethod != http.MethodPut {
		t.Errorf("request = %s %s, want PUT /api/v1/registry-credentials/regcred_1", gotMethod, gotPath)
	}
	if gotBody.Name != "renamed" || gotBody.Username != "new-user" {
		t.Errorf("request body = %+v, want the updated fields", gotBody)
	}
	if gotBody.Password != "" {
		t.Errorf("request body = %+v, want no password when none was given", gotBody)
	}
	if !strings.Contains(stdout, "renamed") {
		t.Errorf("stdout = %q, want confirmation naming the updated credential", stdout)
	}
}

func TestRun_RegistryCredentialsUpdate_RotatesPassword(t *testing.T) {
	srv, gotBody := newRegistryCredentialEchoServer(t, "regcred_1", nil, nil)

	runCLIExpectOK(t, []string{
		"registry-credentials", "update", "regcred_1", "--name", "ghcr-bot", "--registry-host", "ghcr.io",
		"--username", "bot", "--password", "rotated-tok", "--api-url", srv.URL,
	})
	if gotBody.Password != "rotated-tok" {
		t.Errorf("request body = %+v, want the rotated password", gotBody)
	}
}

func TestRun_RegistryCredentialsUpdate_MissingRequiredFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry-credentials", "update", "regcred_1", "--registry-host", "ghcr.io", "--username", "bot"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing-name validation error", stderr.String())
	}
}

func TestRun_RegistryCredentialsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	runCLIExpectOK(t, []string{"registry-credentials", "delete", "regcred_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/registry-credentials/regcred_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/registry-credentials/regcred_1", *gotMethod, *gotPath)
	}
}

func TestRun_RegistryCredentialsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"registry credential not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"registry-credentials", "delete", "regcred_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want the server's not-found message", stderr)
	}
}

func TestRun_RegistryCredentials_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry-credentials", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "registry-credentials list") || !strings.Contains(stdout.String(), "registry-credentials update") {
		t.Errorf("stdout = %q, want usage text listing list and update", stdout.String())
	}
}

func TestRun_RegistryCredentials_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"registry-credentials", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown registry-credentials subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
