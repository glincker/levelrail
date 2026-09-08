package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_CertificatesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []certificateResource{
		{Domain: "app.example.com", Status: "healthy", Issuer: "Test CA", NotAfter: time.Now().Add(30 * 24 * time.Hour)},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"certificates", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/certificates" {
		t.Errorf("path = %q, want /api/v1/certificates", gotPath)
	}
	if !strings.Contains(stdout, "app.example.com") {
		t.Errorf("stdout = %q, want the certificate listed", stdout)
	}
}

func TestRun_CertificatesList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]certificateResource{{Domain: "a.example.com", Status: "expiring_soon"}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"certificates", "list", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"domain": "a.example.com"`) {
		t.Errorf("stdout = %q, want the certificate as JSON", stdout.String())
	}
}

func TestRun_CertificatesList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]certificateResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"certificates", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no certificates") {
		t.Errorf("stdout = %q, want a no-certificates message", stdout.String())
	}
}

func TestRun_Certificates_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"certificates", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
