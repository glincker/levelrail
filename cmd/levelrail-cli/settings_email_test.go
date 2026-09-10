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
	var gotPath string
	srv := newListEchoServer(t, &gotPath, emailSettingsResource{Backend: "smtp", SMTPHost: "smtp.example", SMTPPasswordSet: true})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "email", "get", "--api-url", srv.URL})

	if gotPath != "/api/v1/settings/email" {
		t.Errorf("path = %s, want /api/v1/settings/email", gotPath)
	}
	if !strings.Contains(stdout, "backend:                  smtp") || !strings.Contains(stdout, "smtp_host:                smtp.example") {
		t.Errorf("stdout = %q, want backend/smtp_host lines", stdout)
	}
}

func TestRun_SettingsEmail_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody emailSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		gotBody.SMTPPasswordSet = gotBody.SMTPPassword != ""
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "email", "set",
		"--backend", "smtp", "--smtp-host", "smtp.example", "--smtp-port", "587", "--smtp-from", "a@example.com", "--smtp-password", "shh",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/email", gotMethod, gotPath)
	}
	if gotBody.SMTPHost != "smtp.example" || gotBody.SMTPPort != 587 {
		t.Errorf("request body = %+v, want smtp.example:587", gotBody)
	}
	if !strings.Contains(stdout, "smtp_password_set:        true") {
		t.Errorf("stdout = %q, want smtp_password_set: true", stdout)
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
	if !strings.Contains(stderr.String(), "unknown settings email subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
