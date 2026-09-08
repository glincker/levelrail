package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_StaticSitesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []staticSiteResource{
		{Name: "marketing", Domains: []string{"example.com", "www.example.com"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"static-sites", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/static-sites" {
		t.Errorf("path = %q, want /api/v1/static-sites", gotPath)
	}
	if !strings.Contains(stdout, "marketing") || !strings.Contains(stdout, "example.com") {
		t.Errorf("stdout = %q, want the static site listed", stdout)
	}
}

func TestRun_StaticSitesList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]staticSiteResource{{Name: "docs", Domains: []string{"docs.example.com"}}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"static-sites", "list", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"name": "docs"`) {
		t.Errorf("stdout = %q, want the static site as JSON", stdout.String())
	}
}

func TestRun_StaticSitesList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]staticSiteResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"static-sites", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no static sites") {
		t.Errorf("stdout = %q, want a no-static-sites message", stdout.String())
	}
}

func TestRun_StaticSites_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"static-sites", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
