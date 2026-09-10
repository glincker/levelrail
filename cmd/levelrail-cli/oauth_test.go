package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_OAuth_List(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{
			{Provider: "google", Enabled: true, ClientID: "gid", HasClientSecret: true},
			{Provider: "github", Enabled: false},
			{Provider: "oidc", Enabled: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"oauth", "list", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/oauth" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/oauth", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "google") || !strings.Contains(stdout, "true") {
		t.Errorf("stdout = %q, want a google row", stdout)
	}
}

func TestRun_OAuth_List_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"oauth", "list", "--api-url", srv.URL})

	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_OAuth_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{
			{Provider: "google", Enabled: true, ClientID: "gid", HasClientSecret: true},
			{Provider: "github", Enabled: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"oauth", "get", "google", "--api-url", srv.URL})

	if !strings.Contains(stdout, "provider:           google") {
		t.Errorf("stdout = %q, want provider: google", stdout)
	}
	if !strings.Contains(stdout, "has_client_secret:  true") {
		t.Errorf("stdout = %q, want has_client_secret: true", stdout)
	}
	if strings.Contains(stdout, "client_secret") == false && strings.Contains(stdout, "secret:") {
		t.Errorf("stdout = %q, must never contain the raw secret value", stdout)
	}
}

func TestRun_OAuth_Get_UnknownProvider(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"oauth", "get", "bogus"})

	if !strings.Contains(stderr, "unknown oauth provider") {
		t.Errorf("stderr = %q, want an unknown provider error", stderr)
	}
}

func TestRun_OAuth_Get_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{{Provider: "google", Enabled: true}})
	}))
	defer srv.Close()

	stderr := runCLIExpectValidationError(t, []string{"oauth", "get", "github", "--api-url", srv.URL})

	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want a not found error", stderr)
	}
}

func TestRun_OAuth_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateOAuthProviderSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oauthProviderSettingsResource{
			Provider: "google", Enabled: gotBody.Enabled, ClientID: gotBody.ClientID, HasClientSecret: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"oauth", "set", "google",
		"--client-id", "client-id", "--client-secret", "client-secret",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/oauth/google" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/oauth/google", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.ClientID != "client-id" || gotBody.ClientSecret != "client-secret" {
		t.Errorf("request body = %+v, want Enabled=true ClientID=client-id ClientSecret=client-secret", gotBody)
	}
	if !strings.Contains(stdout, "provider:           google") {
		t.Errorf("stdout = %q, want provider: google", stdout)
	}
}

func TestRun_OAuth_Set_UnknownProvider(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"oauth", "set", "bogus", "--client-id", "x"})

	if !strings.Contains(stderr, "unknown oauth provider") {
		t.Errorf("stderr = %q, want an unknown provider error", stderr)
	}
}

func TestRun_OAuth_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"client_id is required to enable this provider"}`)

	stderr := runCLIExpectAPIError(t, []string{"oauth", "set", "google", "--api-url", srv.URL})

	if !strings.Contains(stderr, "client_id is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_OAuth_Disable(t *testing.T) {
	var putBody updateOAuthProviderSettingsRequest
	var listCalled, putCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			listCalled = true
			_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{
				{Provider: "google", Enabled: true, ClientID: "gid", AllowedEmailDomain: "example.com", HasClientSecret: true},
			})
			return
		}
		putCalled = true
		_ = json.NewDecoder(r.Body).Decode(&putBody)
		_ = json.NewEncoder(w).Encode(oauthProviderSettingsResource{
			Provider: "google", Enabled: false, ClientID: putBody.ClientID, HasClientSecret: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"oauth", "disable", "google", "--api-url", srv.URL})

	if !listCalled || !putCalled {
		t.Fatalf("listCalled=%v putCalled=%v, want both true", listCalled, putCalled)
	}
	if putBody.Enabled {
		t.Errorf("request body Enabled = true, want false")
	}
	if putBody.ClientID != "gid" || putBody.AllowedEmailDomain != "example.com" {
		t.Errorf("request body = %+v, want existing ClientID/AllowedEmailDomain preserved", putBody)
	}
	if putBody.ClientSecret != "" {
		t.Errorf("request body ClientSecret = %q, want empty (kept unchanged)", putBody.ClientSecret)
	}
	if !strings.Contains(stdout, "enabled:            false") {
		t.Errorf("stdout = %q, want enabled: false", stdout)
	}
}

func TestRun_OAuth_Disable_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{})
	}))
	defer srv.Close()

	stderr := runCLIExpectValidationError(t, []string{"oauth", "disable", "google", "--api-url", srv.URL})

	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want a not found error", stderr)
	}
}

func TestRun_OAuth_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"oauth", "-h"})
	if !strings.Contains(stdout, "oauth set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_OAuth_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"oauth"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_OAuth_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"oauth", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
