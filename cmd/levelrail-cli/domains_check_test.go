package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_DomainsCheck(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, domainCheckResource{
		Domain: "app.example.com", Status: "connected", Resolved: true,
		ExpectedHost: "203.0.113.10", ResolvedHosts: []string{"203.0.113.10"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "check", "web", "app.example.com", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/domains/app.example.com/check" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/check", gotPath)
	}
	if !strings.Contains(stdout, "connected") {
		t.Errorf("stdout = %q, want the connected status shown", stdout)
	}
}

func TestRun_DomainsCheck_NotResolving(t *testing.T) {
	srv := newListEchoServer(t, nil, domainCheckResource{
		Domain: "app.example.com", Status: "not_resolving", Resolved: false, ExpectedHost: "203.0.113.10",
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "check", "web", "app.example.com", "--api-url", srv.URL})
	if !strings.Contains(stdout, "not_resolving") {
		t.Errorf("stdout = %q, want the not_resolving status shown", stdout)
	}
}

func TestRun_DomainsCheck_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "check", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DomainsCheck_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, 404, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"domains", "check", "ghost", "app.example.com", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want the server's not-found message", stderr)
	}
}

func TestRun_DomainsCheck_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "check", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "domains check") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
