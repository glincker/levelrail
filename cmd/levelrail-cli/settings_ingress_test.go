package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsIngress_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, ingressSettingsResource{PrimaryDomain: "example.com", ACMEEnabled: true, ACMEEmail: "ops@example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "get", "--api-url", srv.URL})

	if gotPath != "/api/v1/settings/ingress" {
		t.Errorf("path = %s, want /api/v1/settings/ingress", gotPath)
	}
	if !strings.Contains(stdout, "primary_domain:     example.com") || !strings.Contains(stdout, "acme_enabled:       true") {
		t.Errorf("stdout = %q, want primary_domain/acme_enabled lines", stdout)
	}
}

func TestRun_SettingsIngress_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody ingressSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "ingress", "set",
		"--primary-domain", "example.com", "--acme-enabled", "--acme-email", "ops@example.com",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ingress" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ingress", gotMethod, gotPath)
	}
	if gotBody.PrimaryDomain != "example.com" || !gotBody.ACMEEnabled || gotBody.ACMEEmail != "ops@example.com" {
		t.Errorf("request body = %+v, want example.com/true/ops@example.com", gotBody)
	}
	if !strings.Contains(stdout, "primary_domain:     example.com") {
		t.Errorf("stdout = %q, want primary_domain line", stdout)
	}
}

func TestRun_SettingsIngress_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"acme_email is required when acme_enabled is true"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "ingress", "set", "--acme-enabled", "--api-url", srv.URL})
	if !strings.Contains(stderr, "acme_email is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_SettingsIngress_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "-h"})
	if !strings.Contains(stdout, "settings ingress set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_SettingsIngress_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "ingress", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown settings ingress subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
