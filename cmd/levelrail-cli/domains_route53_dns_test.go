package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsRoute53DNS_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(route53DNSResource{Enabled: true, Region: "us-east-1", HasAccessKeyID: true, HasSecretAccessKey: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "route53-dns", "get", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/route53-dns" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/route53-dns", gotMethod, gotPath)
	}
	if !strings.Contains(stdout.String(), "enabled:               true") {
		t.Errorf("stdout = %q, want enabled: true", stdout.String())
	}
}

func TestRun_DomainsRoute53DNS_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateRoute53DNSRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(route53DNSResource{Enabled: true, HasAccessKeyID: true, HasSecretAccessKey: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"domains", "route53-dns", "set",
		"--aws-access-key-id", "AKIA-test",
		"--aws-secret-access-key", "shh",
		"--aws-region", "us-east-1",
		"--hosted-zone-id", "Z123",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/route53-dns" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/route53-dns", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.AccessKeyID != "AKIA-test" || gotBody.SecretAccessKey != "shh" || gotBody.Region != "us-east-1" || gotBody.HostedZoneID != "Z123" {
		t.Errorf("request body = %+v, want Enabled=true AccessKeyID=AKIA-test SecretAccessKey=shh Region=us-east-1 HostedZoneID=Z123", gotBody)
	}
}

func TestRun_DomainsRoute53DNS_Set_RequiresCredentials(t *testing.T) {
	tests := [][]string{
		{"domains", "route53-dns", "set"},
		{"domains", "route53-dns", "set", "--aws-access-key-id", "only-one-half"},
		{"domains", "route53-dns", "set", "--aws-secret-access-key", "only-the-other-half"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		if got != exitUsage {
			t.Errorf("args %v: exit = %d, want %d", args, got, exitUsage)
		}
	}
}

func TestRun_DomainsRoute53DNS_Clear(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(route53DNSResource{Enabled: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "route53-dns", "clear", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/route53-dns" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/route53-dns", gotMethod, gotPath)
	}
	if !strings.Contains(stdout.String(), "enabled:               false") {
		t.Errorf("stdout = %q, want enabled: false", stdout.String())
	}
}

func TestRun_DomainsRoute53DNS_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "route53-dns", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
