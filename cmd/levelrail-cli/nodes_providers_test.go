package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubNodeProviderToken swaps readNodeProviderToken for the duration of
// the test, the same pattern importPromptFn's own tests already use for
// its own stdin/terminal-reading seam.
func stubNodeProviderToken(t *testing.T, token string, err error) {
	t.Helper()
	orig := readNodeProviderToken
	readNodeProviderToken = func(io.Writer) (string, error) { return token, err }
	t.Cleanup(func() { readNodeProviderToken = orig })
}

// stubNodeProviderSecret is stubNodeProviderToken's sibling for
// readNodeProviderSecret (the AWS --secret-access-key stdin/prompt
// fallback).
func stubNodeProviderSecret(t *testing.T, secret string, err error) {
	t.Helper()
	orig := readNodeProviderSecret
	readNodeProviderSecret = func(io.Writer) (string, error) { return secret, err }
	t.Cleanup(func() { readNodeProviderSecret = orig })
}

func TestRun_NodesProvidersList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]nodeProviderResource{
			{Provider: "hetzner", HasToken: true},
			{Provider: "digitalocean", HasToken: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "providers", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/node-providers" {
		t.Errorf("path = %q, want /api/v1/node-providers", gotPath)
	}
	if !strings.Contains(stdout, "hetzner") || !strings.Contains(stdout, "digitalocean") {
		t.Errorf("stdout = %q, want both providers listed", stdout)
	}
}

func TestRun_NodesProvidersSetCredential(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "providers", "set-credential", "--provider", "hetzner", "--provider-token", "secret", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/node-providers" {
		t.Errorf("request = %s %s, want POST /api/v1/node-providers", gotMethod, gotPath)
	}
	if gotBody.Provider != "hetzner" || gotBody.Token != "secret" {
		t.Errorf("request body = %+v", gotBody)
	}
	if !strings.Contains(stdout, "hetzner") {
		t.Errorf("stdout = %q, want the provider name", stdout)
	}
}

func TestRun_NodesProvidersSetCredential_MissingProvider(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"nodes", "providers", "set-credential", "--provider-token", "secret"})
	if !strings.Contains(stderr, "--provider is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_NodesProvidersSetCredential_NoTokenAnywhere(t *testing.T) {
	stubNodeProviderToken(t, "", nil)
	stderr := runCLIExpectValidationError(t, []string{"nodes", "providers", "set-credential", "--provider", "hetzner"})
	if !strings.Contains(stderr, "a provider token is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_NodesProvidersSetCredential_TokenFromStdinOrPrompt(t *testing.T) {
	stubNodeProviderToken(t, "piped-secret", nil)
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"nodes", "providers", "set-credential", "--provider", "hetzner", "--api-url", srv.URL})
	if gotBody.Token != "piped-secret" {
		t.Errorf("Token = %q, want piped-secret", gotBody.Token)
	}
}

func TestRun_NodesProvidersSetCredential_AWS(t *testing.T) {
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "providers", "set-credential", "--provider", "aws",
		"--provider-token", "AKIA...", "--secret-access-key", "shh", "--region", "eu-west-1",
		"--api-url", srv.URL,
	})
	if gotBody.Provider != "aws" || gotBody.Token != "AKIA..." || gotBody.SecretAccessKey != "shh" || gotBody.Region != "eu-west-1" {
		t.Errorf("request body = %+v", gotBody)
	}
}

func TestRun_NodesProvidersSetCredential_AWS_MissingSecretAccessKey(t *testing.T) {
	stubNodeProviderSecret(t, "", nil)
	stderr := runCLIExpectValidationError(t, []string{
		"nodes", "providers", "set-credential", "--provider", "aws", "--provider-token", "AKIA...",
	})
	if !strings.Contains(stderr, "--secret-access-key") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestRun_NodesProvidersSetCredential_AWS_SecretFromStdinOrPrompt covers
// the fix for the secret access key being exposed in CLI arguments: like
// --provider-token, --secret-access-key now falls back to a protected
// stdin/prompt read when the flag is omitted, instead of being required as
// a flag value visible in shell history and the process list.
func TestRun_NodesProvidersSetCredential_AWS_SecretFromStdinOrPrompt(t *testing.T) {
	stubNodeProviderSecret(t, "piped-secret", nil)
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "providers", "set-credential", "--provider", "aws",
		"--provider-token", "AKIA...", "--api-url", srv.URL,
	})
	if gotBody.SecretAccessKey != "piped-secret" {
		t.Errorf("SecretAccessKey = %q, want piped-secret", gotBody.SecretAccessKey)
	}
}

func TestRun_NodesProvidersSetCredential_AWS_AmbientCredentialsSkipsKeyRequirement(t *testing.T) {
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "providers", "set-credential", "--provider", "aws", "--use-ambient-credentials",
		"--api-url", srv.URL,
	})
	if !gotBody.UseAmbientCredentials {
		t.Errorf("UseAmbientCredentials = false, want true")
	}
}

func TestRun_NodesProviders_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "providers", "-h"})
	if !strings.Contains(stdout, "nodes providers") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
