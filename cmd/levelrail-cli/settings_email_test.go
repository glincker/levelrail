package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsEmail_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{
			Backend: "smtp", SMTPHost: "smtp.example.com", SMTPPort: 587,
			SMTPFrom: "noreply@example.com", SMTPPasswordSet: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "email", "get", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/email", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "backend:                   smtp") || !strings.Contains(stdout, "smtp_password_set:         true") {
		t.Errorf("stdout = %q, want backend and smtp_password_set lines", stdout)
	}
}

// TestRun_SettingsEmail_Get_NeverPrintsSecretValue confirms that even
// though EmailSettingsResource (the real GET wire shape) has no field
// that could carry a plaintext credential, the CLI's own rendering never
// invents one: only the *_set booleans appear, matching what the real
// API handler (internal/api/email_settings.go's toEmailSettingsResource)
// actually returns.
func TestRun_SettingsEmail_Get_NeverPrintsSecretValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{
			Backend: "ses", SESRegion: "us-east-1", SESAccessKeyID: "AKIA-test", SESSecretAccessKeySet: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "email", "get", "--json", "--api-url", srv.URL})

	if strings.Contains(stdout, "ses_secret_access_key\":") {
		t.Errorf("stdout = %q, want no raw ses_secret_access_key field, only ses_secret_access_key_set", stdout)
	}
	if !strings.Contains(stdout, "ses_secret_access_key_set") {
		t.Errorf("stdout = %q, want ses_secret_access_key_set", stdout)
	}
}

func TestRun_SettingsEmail_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateEmailSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{Backend: "smtp", SMTPPasswordSet: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "email", "set",
		"--backend", "smtp",
		"--smtp-host", "smtp.example.com",
		"--smtp-port", "587",
		"--smtp-from", "noreply@example.com",
		"--smtp-password", "hunter2",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/email", gotMethod, gotPath)
	}
	if gotBody.Backend != "smtp" || gotBody.SMTPHost != "smtp.example.com" || gotBody.SMTPPort != 587 || gotBody.SMTPPassword != "hunter2" {
		t.Errorf("request body = %+v, want the smtp fields including the password", gotBody)
	}
	if strings.Contains(stdout, "hunter2") {
		t.Errorf("stdout = %q, want the password never echoed back", stdout)
	}
}

func TestRun_SettingsEmail_Set_Omitted_KeepsStoredCredential(t *testing.T) {
	var gotBody updateEmailSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{Backend: "smtp", SMTPPasswordSet: true})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"settings", "email", "set", "--backend", "smtp", "--smtp-host", "h", "--smtp-from", "f", "--smtp-port", "25", "--api-url", srv.URL})

	if gotBody.SMTPPassword != "" {
		t.Errorf("request body SMTPPassword = %q, want empty (kept unchanged)", gotBody.SMTPPassword)
	}
}

func TestRun_SettingsEmail_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"smtp_host is required when backend is smtp"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "email", "set", "--backend", "smtp", "--api-url", srv.URL})

	if !strings.Contains(stderr, "smtp_host is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_SettingsEmail_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "email", "-h"})
	if !strings.Contains(stdout, "settings email set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_SettingsEmail_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "email", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
