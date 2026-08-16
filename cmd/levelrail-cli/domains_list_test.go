package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domainResource{
			{Domain: "app.example.com", ServiceName: "web"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/domains" {
		t.Errorf("path = %q, want /api/v1/domains", gotPath)
	}
	if !strings.Contains(stdout.String(), "app.example.com") {
		t.Errorf("stdout = %q, want the domain listed", stdout.String())
	}
}

func TestRun_DomainsList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domainResource{{Domain: "a.example.com", ServiceName: "web"}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "list", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"domain": "a.example.com"`) {
		t.Errorf("stdout = %q, want domains as JSON", stdout.String())
	}
}

func TestRun_DomainsList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domainResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no domains") {
		t.Errorf("stdout = %q, want a no-domains message", stdout.String())
	}
}

func TestRun_Domains_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "bogus"}, &stdout, &stderr, envMap(nil))
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
