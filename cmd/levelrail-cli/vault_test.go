package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Vault_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{
			Enabled: true, Address: "https://vault:8200", AuthMethod: "token",
			MountPath: "secret", HasCredential: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"vault", "get", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/vault" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/vault", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:        true") || !strings.Contains(stdout, "address:        https://vault:8200") {
		t.Errorf("stdout = %q, want enabled and address lines", stdout)
	}
}

func TestRun_Vault_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{Enabled: true, AuthMethod: "token", MountPath: "secret"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"vault", "get", "--json", "--api-url", srv.URL})

	var got vaultSettingsResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if !got.Enabled || got.AuthMethod != "token" {
		t.Errorf("got = %+v, want Enabled=true AuthMethod=token", got)
	}
}

func TestRun_Vault_Get_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"vault", "get", "--api-url", srv.URL})

	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_Vault_Set_TokenAuth(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateVaultSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{Enabled: true, Address: "https://vault:8200", AuthMethod: "token", MountPath: "secret", HasCredential: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"vault", "set", "--address", "https://vault:8200", "--auth-method", "token", "--vault-token", "my-vault-token", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/vault" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/vault", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.Address != "https://vault:8200" || gotBody.AuthMethod != "token" || gotBody.Credential != "my-vault-token" {
		t.Errorf("request body = %+v, want Enabled=true Address=https://vault:8200 AuthMethod=token Credential=my-vault-token", gotBody)
	}
	if !strings.Contains(stdout, "has_credential: true") {
		t.Errorf("stdout = %q, want has_credential: true", stdout)
	}
}

func TestRun_Vault_Set_AppRoleAuth(t *testing.T) {
	var gotBody updateVaultSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{Enabled: true, AuthMethod: "approle", RoleID: "role-123", MountPath: "secret", HasCredential: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"vault", "set", "--address", "https://vault:8200", "--auth-method", "approle", "--role-id", "role-123", "--secret-id", "a-secret-id", "--api-url", srv.URL})

	if gotBody.AuthMethod != "approle" || gotBody.RoleID != "role-123" || gotBody.Credential != "a-secret-id" {
		t.Errorf("request body = %+v, want AuthMethod=approle RoleID=role-123 Credential=a-secret-id", gotBody)
	}
}

func TestRun_Vault_Set_Disable(t *testing.T) {
	var gotBody updateVaultSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{Enabled: false, AuthMethod: "token", MountPath: "secret"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"vault", "set", "--enabled=false", "--api-url", srv.URL})

	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false")
	}
	if gotBody.Credential != "" {
		t.Errorf("request body Credential = %q, want empty (kept unchanged)", gotBody.Credential)
	}
}

func TestRun_Vault_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"credential is required the first time vault is enabled"}`)

	stderr := runCLIExpectAPIError(t, []string{"vault", "set", "--address", "https://vault:8200", "--api-url", srv.URL})

	if !strings.Contains(stderr, "credential is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_Vault_Disconnect(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSettingsResource{Enabled: false, AuthMethod: "token", MountPath: "secret"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"vault", "disconnect", "--api-url", srv.URL})

	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/vault" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/vault", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:        false") {
		t.Errorf("stdout = %q, want enabled: false", stdout)
	}
}

func TestRun_Vault_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"vault", "-h"})
	if !strings.Contains(stdout, "vault set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_Vault_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"vault"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_Vault_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"vault", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
