package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsCloudflareDNS_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareDNSResource{Enabled: true, HasToken: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "cloudflare-dns", "get", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/cloudflare-dns" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/cloudflare-dns", gotMethod, gotPath)
	}
	if !strings.Contains(stdout.String(), "enabled:   true") {
		t.Errorf("stdout = %q, want enabled: true", stdout.String())
	}
}

func TestRun_DomainsCloudflareDNS_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateCloudflareDNSRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareDNSResource{Enabled: true, HasToken: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "cloudflare-dns", "set", "--cf-api-token", "cf-test-token", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/cloudflare-dns" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/cloudflare-dns", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.Token != "cf-test-token" {
		t.Errorf("request body = %+v, want Enabled=true Token=cf-test-token", gotBody)
	}
}

func TestRun_DomainsCloudflareDNS_Set_RequiresToken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "cloudflare-dns", "set"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_DomainsCloudflareDNS_Clear(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareDNSResource{Enabled: false, HasToken: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "cloudflare-dns", "clear", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/cloudflare-dns" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/cloudflare-dns", gotMethod, gotPath)
	}
	if !strings.Contains(stdout.String(), "enabled:   false") {
		t.Errorf("stdout = %q, want enabled: false", stdout.String())
	}
}

func TestRun_DomainsCloudflareDNS_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "cloudflare-dns", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
