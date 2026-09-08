package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTLSCertFixtureFiles writes fake cert/key PEM text to two files
// under t.TempDir() and returns their paths, the fixture every
// --cert-file/--key-file test below needs: this command reads PEM from
// disk, never accepts it as a command-line argument (see
// domains_tls_cert.go's own doc comment on why).
func writeTLSCertFixtureFiles(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, []byte("fake-cert-pem"), 0o600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("fake-key-pem"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return certFile, keyFile
}

func TestRun_DomainsTLSCertSet(t *testing.T) {
	certFile, keyFile := writeTLSCertFixtureFiles(t)

	var gotMethod, gotPath string
	var gotBody setDomainTLSCertRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainTLSCertResource{Domain: "app.example.com", Enabled: true, UploadedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2027-01-01T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "set", "web", "app.example.com", "--cert-file", certFile, "--key-file", keyFile, "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/tls-cert" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/tls-cert", gotPath)
	}
	if gotBody.Cert != "fake-cert-pem" || gotBody.Key != "fake-key-pem" {
		t.Errorf("request body = %+v, want the file contents verbatim", gotBody)
	}
	if !strings.Contains(stdout.String(), `"enabled": true`) {
		t.Errorf("stdout = %q, want the tls cert state as JSON", stdout.String())
	}
	if strings.Contains(stdout.String(), "fake-cert-pem") || strings.Contains(stdout.String(), "fake-key-pem") {
		t.Errorf("stdout = %q, must never echo the certificate or key", stdout.String())
	}
}

func TestRun_DomainsTLSCertSet_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "set", "web", "app.example.com"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires --cert-file and --key-file") {
		t.Errorf("stderr = %q, want a missing-flags usage error", stderr.String())
	}
}

func TestRun_DomainsTLSCertSet_MissingArgs(t *testing.T) {
	certFile, keyFile := writeTLSCertFixtureFiles(t)
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "set", "web", "--cert-file", certFile, "--key-file", keyFile}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}

func TestRun_DomainsTLSCertSet_UnreadableCertFile(t *testing.T) {
	_, keyFile := writeTLSCertFixtureFiles(t)
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "set", "web", "app.example.com", "--cert-file", "/nonexistent/cert.pem", "--key-file", keyFile}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a failure for an unreadable cert file", got)
	}
	if !strings.Contains(stderr.String(), "read cert file") {
		t.Errorf("stderr = %q, want a cert-file read error", stderr.String())
	}
}

func TestRun_DomainsTLSCertGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/tls-cert" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/tls-cert", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainTLSCertResource{Domain: "app.example.com", Enabled: true, UploadedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2027-01-01T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "get", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "BYO certificate uploaded") {
		t.Errorf("stdout = %q, want the enabled status line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "expires_at:  2027-01-01T00:00:00Z") {
		t.Errorf("stdout = %q, want the expiry line", stdout.String())
	}
}

func TestRun_DomainsTLSCertGet_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainTLSCertResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "get", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "automatic (ACME/internal)") {
		t.Errorf("stdout = %q, want the automatic-issuance status before any cert is uploaded", stdout.String())
	}
}

func TestRun_DomainsTLSCertClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainTLSCertResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "clear", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/tls-cert" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/tls-cert", gotPath)
	}
	if !strings.Contains(stdout.String(), `tls certificate removed for domain "app.example.com"`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout.String())
	}
}

func TestRun_DomainsTLSCert_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "domains tls-cert") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_DomainsTLSCert_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "tls-cert", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown domains tls-cert subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
