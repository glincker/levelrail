package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsMaintenanceSet(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainMaintenanceResource{Domain: "app.example.com", Enabled: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "maintenance", "set", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/maintenance" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/maintenance", gotPath)
	}
	if !strings.Contains(stdout.String(), "status: enabled") {
		t.Errorf("stdout = %q, want the enabled status line", stdout.String())
	}
}

func TestRun_DomainsMaintenanceSet_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "maintenance", "set", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}

func TestRun_DomainsMaintenanceGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/maintenance" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/maintenance", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainMaintenanceResource{Domain: "app.example.com", Enabled: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "maintenance", "get", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status: disabled") {
		t.Errorf("stdout = %q, want the disabled status line", stdout.String())
	}
}

func TestRun_DomainsMaintenanceClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, domainMaintenanceResource{Domain: "app.example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "maintenance", "clear", "web", "app.example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/maintenance" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/maintenance", gotPath)
	}
	if !strings.Contains(stdout, `maintenance mode disabled for domain "app.example.com"`) {
		t.Errorf("stdout = %q, want a disable confirmation", stdout)
	}
}

func TestRun_DomainsMaintenanceClear_JSON(t *testing.T) {
	srv := newEchoServer(t, nil, nil, domainMaintenanceResource{Domain: "app.example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "maintenance", "clear", "web", "app.example.com", "--api-url", srv.URL, "--json"})
	var m domainMaintenanceResource
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if m.Domain != "app.example.com" {
		t.Errorf("domain = %q, want app.example.com", m.Domain)
	}
}

func TestRun_DomainsMaintenance_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "maintenance", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "domains maintenance") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_DomainsMaintenance_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "maintenance", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown domains maintenance subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
