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
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingressSettingsResource{PrimaryDomain: "app.example.com", ACMEEnabled: true, ACMEEmail: "ops@example.com"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "get", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/ingress" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/ingress", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "primary_domain:     app.example.com") || !strings.Contains(stdout, "acme_enabled:       true") {
		t.Errorf("stdout = %q, want primary_domain and acme_enabled lines", stdout)
	}
}

func TestRun_SettingsIngress_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody ingressSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"settings", "ingress", "set",
		"--primary-domain", "app.example.com",
		"--acme-enabled",
		"--acme-email", "ops@example.com",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ingress" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ingress", gotMethod, gotPath)
	}
	if gotBody.PrimaryDomain != "app.example.com" || !gotBody.ACMEEnabled || gotBody.ACMEEmail != "ops@example.com" {
		t.Errorf("request body = %+v, want the primary domain and acme fields", gotBody)
	}
}

func TestRun_SettingsIngress_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"acme_email is required when acme_enabled is true"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "ingress", "set", "--acme-enabled", "--api-url", srv.URL})

	if !strings.Contains(stderr, "acme_email is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_SettingsIngress_Check_Configured(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingressDomainCheckResource{
			Configured: true, Domain: "app.example.com", ExpectedHost: "203.0.113.5", Resolved: true, Status: "ok",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "check", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/ingress/check" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/ingress/check", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "configured: true") || !strings.Contains(stdout, "status:        ok") {
		t.Errorf("stdout = %q, want configured and status lines", stdout)
	}
}

func TestRun_SettingsIngress_Check_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingressDomainCheckResource{Configured: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "check", "--api-url", srv.URL})

	if !strings.Contains(stdout, "configured: false") {
		t.Errorf("stdout = %q, want configured: false", stdout)
	}
	if strings.Contains(stdout, "domain:") {
		t.Errorf("stdout = %q, want no domain line when not configured", stdout)
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
}
