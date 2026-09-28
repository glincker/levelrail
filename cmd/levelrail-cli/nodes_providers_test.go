package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestRun_NodesProvidersSetCredential_MissingFlags(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"nodes", "providers", "set-credential", "--provider", "hetzner"})
	if !strings.Contains(stderr, "--provider-token is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_NodesProviders_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "providers", "-h"})
	if !strings.Contains(stdout, "nodes providers") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
