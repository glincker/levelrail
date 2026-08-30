package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsBasicAuthSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDomainBasicAuthRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainBasicAuthResource{Domain: "app.example.com", Enabled: true, Username: "operator", HasPassword: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "set", "web", "app.example.com", "--username", "operator", "--password", "hunter2", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/auth" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/auth", gotPath)
	}
	if gotBody.Username != "operator" || gotBody.Password != "hunter2" {
		t.Errorf("request body = %+v, want username=operator password=hunter2", gotBody)
	}
	if !strings.Contains(stdout.String(), `"username": "operator"`) {
		t.Errorf("stdout = %q, want the auth state as JSON", stdout.String())
	}
	if strings.Contains(stdout.String(), "hunter2") {
		t.Errorf("stdout = %q, must never echo the password", stdout.String())
	}
}

func TestRun_DomainsBasicAuthSet_MissingUsername(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "set", "web", "app.example.com", "--password", "hunter2"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires --username") {
		t.Errorf("stderr = %q, want a missing --username usage error", stderr.String())
	}
}

func TestRun_DomainsBasicAuthSet_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "set", "web", "--username", "operator", "--password", "hunter2"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}

func TestRun_DomainsBasicAuthGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/auth" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/auth", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainBasicAuthResource{Domain: "app.example.com", Enabled: true, Username: "operator", HasPassword: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "get", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status:      enabled") {
		t.Errorf("stdout = %q, want the status line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "username:    operator") {
		t.Errorf("stdout = %q, want the username line", stdout.String())
	}
}

func TestRun_DomainsBasicAuthGet_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainBasicAuthResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "get", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status:      disabled") {
		t.Errorf("stdout = %q, want disabled before any basic auth is set", stdout.String())
	}
}

func TestRun_DomainsBasicAuthClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainBasicAuthResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "clear", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/auth" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/auth", gotPath)
	}
	if !strings.Contains(stdout.String(), `basic auth removed for domain "app.example.com"`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout.String())
	}
}

func TestRun_DomainsBasicAuth_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "domains basic-auth") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_DomainsBasicAuth_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "basic-auth", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown domains basic-auth subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
