package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsOAuth_List(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]oauthProviderSettingsResource{
			{Provider: "google", Enabled: true, ClientID: "google-client", HasClientSecret: true},
			{Provider: "github", Enabled: false},
			{Provider: "oidc", Enabled: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "oauth", "list", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/oauth" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/oauth", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "google") || !strings.Contains(stdout, "github") || !strings.Contains(stdout, "oidc") {
		t.Errorf("stdout = %q, want all three providers listed", stdout)
	}
}

func TestRun_SettingsOAuth_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateOAuthProviderSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oauthProviderSettingsResource{Provider: "google", Enabled: true, ClientID: "id-1", HasClientSecret: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "oauth", "set", "google",
		"--client-id", "id-1",
		"--client-secret", "top-secret",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/oauth/google" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/oauth/google", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.ClientID != "id-1" || gotBody.ClientSecret != "top-secret" {
		t.Errorf("request body = %+v, want Enabled=true ClientID=id-1 ClientSecret=top-secret", gotBody)
	}
	if strings.Contains(stdout, "top-secret") {
		t.Errorf("stdout = %q, want the client secret never echoed back", stdout)
	}
}

func TestRun_SettingsOAuth_Set_Omitted_KeepsStoredSecret(t *testing.T) {
	var gotBody updateOAuthProviderSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oauthProviderSettingsResource{Provider: "github", Enabled: true, HasClientSecret: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"settings", "oauth", "set", "github", "--client-id", "id-2", "--api-url", srv.URL})

	if gotBody.ClientSecret != "" {
		t.Errorf("request body ClientSecret = %q, want empty (kept unchanged)", gotBody.ClientSecret)
	}
}

func TestRun_SettingsOAuth_Set_RequiresProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "oauth", "set", "--client-id", "id-1"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_SettingsOAuth_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"client_secret is required the first time a provider is enabled"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "oauth", "set", "google", "--client-id", "id-1", "--api-url", srv.URL})

	if !strings.Contains(stderr, "client_secret is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_SettingsOAuth_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "oauth", "-h"})
	if !strings.Contains(stdout, "settings oauth set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_SettingsOAuth_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "oauth", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
