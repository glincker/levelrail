package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Email_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{
			Backend: "smtp", SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPFrom: "a@example.com", SMTPPasswordSet: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"email", "get", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/email", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "backend:                     smtp") {
		t.Errorf("stdout = %q, want backend: smtp", stdout)
	}
	if !strings.Contains(stdout, "smtp_password_set:           true") {
		t.Errorf("stdout = %q, want smtp_password_set: true", stdout)
	}
	if strings.Contains(stdout, "hunter2") {
		t.Errorf("stdout = %q, must never contain a raw credential value", stdout)
	}
}

func TestRun_Email_Get_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{Backend: ""})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"email", "get", "--api-url", srv.URL})

	if !strings.Contains(stdout, "backend:                     (disabled)") {
		t.Errorf("stdout = %q, want backend: (disabled)", stdout)
	}
}

func TestRun_Email_Get_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"email", "get", "--api-url", srv.URL})

	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_Email_Set_SMTP(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody emailSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emailSettingsResource{
			Backend: gotBody.Backend, SMTPHost: gotBody.SMTPHost, SMTPPasswordSet: gotBody.SMTPPassword != "",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"email", "set", "--backend", "smtp",
		"--smtp-host", "smtp.example.com", "--smtp-port", "587",
		"--smtp-username", "bot", "--smtp-from", "noreply@example.com",
		"--smtp-password", "hunter2",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/email", gotMethod, gotPath)
	}
	if gotBody.Backend != "smtp" || gotBody.SMTPHost != "smtp.example.com" || gotBody.SMTPPort != 587 {
		t.Errorf("request body = %+v, want smtp backend/host/port", gotBody)
	}
	if gotBody.SMTPPassword != "hunter2" {
		t.Errorf("request body SMTPPassword = %q, want hunter2", gotBody.SMTPPassword)
	}
	if !strings.Contains(stdout, "smtp_password_set:           true") {
		t.Errorf("stdout = %q, want smtp_password_set: true", stdout)
	}
	if strings.Contains(stdout, "hunter2") {
		t.Errorf("stdout = %q, must never echo the raw password back", stdout)
	}
}

func TestRun_Email_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"smtp_host is required when backend is smtp"}`)

	stderr := runCLIExpectAPIError(t, []string{"email", "set", "--backend", "smtp", "--api-url", srv.URL})

	if !strings.Contains(stderr, "smtp_host is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_Email_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"email", "-h"})
	if !strings.Contains(stdout, "email set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_Email_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"email"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_Email_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"email", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
