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
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []oauthProviderSettingsResource{
		{Provider: "google", Enabled: true, ClientID: "abc", HasClientSecret: true},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "oauth", "list", "--api-url", srv.URL})

	if gotPath != "/api/v1/settings/oauth" {
		t.Errorf("path = %s, want /api/v1/settings/oauth", gotPath)
	}
	if !strings.Contains(stdout, "google") || !strings.Contains(stdout, "abc") {
		t.Errorf("stdout = %q, want google/abc listed", stdout)
	}
}

func TestRun_SettingsOAuth_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []oauthProviderSettingsResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "oauth", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no oauth providers") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_SettingsOAuth_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody updateOAuthProviderSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oauthProviderSettingsResource{Provider: "github", Enabled: gotBody.Enabled, ClientID: gotBody.ClientID, HasClientSecret: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "oauth", "set", "github",
		"--client-id", "abc", "--client-secret", "shh",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/oauth/github" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/oauth/github", gotMethod, gotPath)
	}
	if gotBody.ClientID != "abc" || gotBody.ClientSecret != "shh" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want ClientID=abc ClientSecret=shh Enabled=true", gotBody)
	}
	if !strings.Contains(stdout, "provider:             github") {
		t.Errorf("stdout = %q, want provider line", stdout)
	}
}

func TestRun_SettingsOAuth_Set_MissingProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "oauth", "set", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-arg usage error", stderr.String())
	}
}

func TestRun_SettingsOAuth_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"client_id is required to enable this provider"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "oauth", "set", "google", "--api-url", srv.URL})
	if !strings.Contains(stderr, "client_id is required") {
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
	if !strings.Contains(stderr.String(), "unknown settings oauth subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_Settings_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown settings subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_Settings_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_Settings_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "-h"})
	if !strings.Contains(stdout, "settings oauth") || !strings.Contains(stdout, "settings email") || !strings.Contains(stdout, "settings ingress") {
		t.Errorf("stdout = %q, want all three settings resources listed", stdout)
	}
}
