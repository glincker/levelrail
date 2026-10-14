package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_RegistryCredentialsAddWithVendorPreset(t *testing.T) {
	var gotPath string
	var gotBody createRegistryCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registryCredentialResource{ID: "regcred_1", Name: gotBody.Name, RegistryHost: gotBody.RegistryHost})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"registry", "credentials", "add", "--vendor", "ecr", "--registry-host", "123.dkr.ecr.eu-west-1.amazonaws.com", "--password", "ecr-token", "--api-url", srv.URL})
	if gotPath != "/api/v1/registry-credentials" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody.Username != "AWS" || gotBody.Name != "ecr" || gotBody.ExpiresAt == nil {
		t.Errorf("body = %+v, want AWS username, vendor name and a derived expiry", gotBody)
	}

	runCLIExpectOK(t, []string{"registry", "credentials", "add", "--vendor", "ghcr", "--username", "octo", "--password", "pat", "--api-url", srv.URL})
	if gotBody.RegistryHost != "ghcr.io" {
		t.Errorf("ghcr host = %q", gotBody.RegistryHost)
	}
}

func TestRun_RegistryCredentialsAddRequiresHostForTenantVendors(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"registry", "credentials", "add", "--vendor", "harbor", "--username", "u", "--password", "p"})
	if !strings.Contains(stderr, "--registry-host is required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_PackageRegistriesAdd(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "pkgreg_1", "name": "corp", "build_secret_id": "npmrc"})
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"registry", "package-registries", "add", "--ecosystem", "npm", "--name", "corp", "--url", "https://npm.example.com", "--secret", "tok", "--api-url", srv.URL})
	if gotBody["ecosystem"] != "npm" || gotBody["secret"] != "tok" {
		t.Errorf("body = %v", gotBody)
	}
	if !strings.Contains(out, "id=npmrc") {
		t.Errorf("stdout = %q", out)
	}
}
