package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_DomainsDNSList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/dns-records" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/dns-records", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiclient.DNSRecordsResponse{
			Domain: "app.example.com", Provider: "cloudflare", Zone: "example.com.",
			Records: []apiclient.DNSRecordResource{
				{Name: "app", Type: "A", Value: "203.0.113.10", TTLSeconds: 300, Status: "resolved"},
			},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "dns", "list", "web", "app.example.com", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"zone": "example.com."`) {
		t.Errorf("stdout = %q, want the dns records result as JSON", stdout.String())
	}
}

func TestRun_DomainsDNSAdd(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody apiclient.DNSRecordResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiclient.DNSRecordsResponse{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"domains", "dns", "add", "web", "app.example.com",
		"--type", "A", "--name", "www", "--value", "203.0.113.5", "--ttl", "600",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/dns-records" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/dns-records", gotPath)
	}
	if gotBody.Type != "A" || gotBody.Name != "www" || gotBody.Value != "203.0.113.5" || gotBody.TTLSeconds != 600 {
		t.Errorf("request body = %+v, want type=A name=www value=203.0.113.5 ttl=600", gotBody)
	}
}

func TestRun_DomainsDNSAdd_MissingRequiredFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "dns", "add", "web", "app.example.com", "--type", "A"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--type and --value are required") {
		t.Errorf("stderr = %q, want a missing --value usage error", stderr.String())
	}
}

func TestRun_DomainsDNSRemove(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody apiclient.DNSRecordResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiclient.DNSRecordsResponse{Domain: "app.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"domains", "dns", "remove", "web", "app.example.com",
		"--type", "A", "--name", "www", "--value", "203.0.113.5",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/dns-records" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/dns-records", gotPath)
	}
	if gotBody.Type != "A" || gotBody.Name != "www" || gotBody.Value != "203.0.113.5" {
		t.Errorf("request body = %+v, want type=A name=www value=203.0.113.5", gotBody)
	}
}

func TestRun_DomainsDNS_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "dns", "list", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}
