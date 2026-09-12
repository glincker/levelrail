package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsWAFSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDomainWAFRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainWAFResource{
			Domain: "app.example.com", WAFEnabled: true, WAFMode: "block",
			RateLimitEnabled: true, RateLimitRPS: 10, RateLimitBurst: 20,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"domains", "waf", "set", "web", "app.example.com",
		"--waf", "--mode", "block", "--rps", "10", "--burst", "20",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/waf" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/waf", gotPath)
	}
	if !gotBody.WAFEnabled || gotBody.WAFMode != "block" || gotBody.RateLimitRPS != 10 || gotBody.RateLimitBurst != 20 {
		t.Errorf("request body = %+v, want waf_enabled=true mode=block rps=10 burst=20", gotBody)
	}
	if !strings.Contains(stdout.String(), `"waf_mode": "block"`) {
		t.Errorf("stdout = %q, want the waf state as JSON", stdout.String())
	}
}

func TestRun_DomainsWAFSet_DefaultModeIsDetect(t *testing.T) {
	var gotBody setDomainWAFRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainWAFResource{Domain: "app.example.com", WAFEnabled: true, WAFMode: "detect"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"domains", "waf", "set", "web", "app.example.com", "--waf",
		"--api-url", srv.URL, "--json",
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if gotBody.WAFMode != "detect" {
		t.Errorf("request body mode = %q, want the flag's own detect default when --mode is not passed", gotBody.WAFMode)
	}
}

func TestRun_DomainsWAFSet_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "waf", "set", "web", "--waf"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-domain usage error", stderr.String())
	}
}

func TestRun_DomainsWAFGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/domains/app.example.com/waf" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/domains/app.example.com/waf", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainWAFResource{Domain: "app.example.com", WAFMode: "detect"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "waf", "get", "web", "app.example.com", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"domain": "app.example.com"`) {
		t.Errorf("stdout = %q, want the waf state as JSON", stdout.String())
	}
}

func TestRun_DomainsWAFClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainWAFResource{Domain: "app.example.com", WAFMode: "detect"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "waf", "clear", "web", "app.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/waf" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/waf", gotPath)
	}
	if !strings.Contains(stdout.String(), "disabled") {
		t.Errorf("stdout = %q, want a human-readable disabled confirmation", stdout.String())
	}
}
