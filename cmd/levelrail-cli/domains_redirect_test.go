package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsRedirectSet(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainRedirectResource{Domain: "www.example.com", Enabled: true, TargetURL: "https://example.com", StatusCode: 301})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "set", "web", "www.example.com", "--target", "https://example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/www.example.com/redirect" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/www.example.com/redirect", gotPath)
	}
	if !strings.Contains(gotBody, `"target_url":"https://example.com"`) {
		t.Errorf("body = %q, want target_url set", gotBody)
	}
	if !strings.Contains(stdout.String(), "target:      https://example.com") {
		t.Errorf("stdout = %q, want the target line", stdout.String())
	}
}

func TestRun_DomainsRedirectSet_Temporary(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainRedirectResource{Domain: "www.example.com", Enabled: true, TargetURL: "https://example.com", StatusCode: 302})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "redirect", "set", "web", "www.example.com", "--target", "https://example.com", "--temporary", "--api-url", srv.URL})
	if !strings.Contains(gotBody, `"status_code":302`) {
		t.Errorf("body = %q, want status_code 302", gotBody)
	}
	if !strings.Contains(stdout, "status_code: 302") {
		t.Errorf("stdout = %q, want status_code 302", stdout)
	}
}

func TestRun_DomainsRedirectSet_ConflictingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "set", "web", "www.example.com", "--target", "https://example.com", "--permanent", "--temporary"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want a mutually-exclusive-flags error", stderr.String())
	}
}

func TestRun_DomainsRedirectSet_MissingTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "set", "web", "www.example.com"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Errorf("stderr = %q, want a missing-target usage error", stderr.String())
	}
}

func TestRun_DomainsRedirectSet_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "set", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}

func TestRun_DomainsRedirectGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/www.example.com/redirect" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/www.example.com/redirect", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainRedirectResource{Domain: "www.example.com", Enabled: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "get", "web", "www.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "no redirect configured") {
		t.Errorf("stdout = %q, want the no-redirect status line", stdout.String())
	}
}

func TestRun_DomainsRedirectClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, domainRedirectResource{Domain: "www.example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "redirect", "clear", "web", "www.example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/www.example.com/redirect" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/www.example.com/redirect", gotPath)
	}
	if !strings.Contains(stdout, `redirect removed for domain "www.example.com"`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_DomainsRedirectClear_JSON(t *testing.T) {
	srv := newEchoServer(t, nil, nil, domainRedirectResource{Domain: "www.example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "redirect", "clear", "web", "www.example.com", "--api-url", srv.URL, "--json"})
	var rd domainRedirectResource
	if err := json.Unmarshal([]byte(stdout), &rd); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if rd.Domain != "www.example.com" {
		t.Errorf("domain = %q, want www.example.com", rd.Domain)
	}
}

func TestRun_DomainsRedirect_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "domains redirect") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_DomainsRedirect_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "redirect", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown domains redirect subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
