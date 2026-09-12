package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_StaticSites_List(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []staticSiteResource{
		{Name: "marketing", Domains: []string{"example.com", "www.example.com"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"static-sites", "list", "--api-url", srv.URL})

	if gotPath != "/api/v1/static-sites" {
		t.Errorf("path = %s, want /api/v1/static-sites", gotPath)
	}
	if !strings.Contains(stdout, "marketing") || !strings.Contains(stdout, "example.com") {
		t.Errorf("stdout = %q, want marketing/example.com listed", stdout)
	}
}

func TestRun_StaticSites_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []staticSiteResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"static-sites", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no static sites") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_StaticSites_List_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"static-sites", "list", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_StaticSites_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"static-sites", "-h"})
	if !strings.Contains(stdout, "static-sites list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_StaticSites_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"static-sites", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown static-sites subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_StaticSites_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"static-sites"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
