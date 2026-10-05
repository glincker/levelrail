package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsIngressHTTPS(t *testing.T) {
	var gotReq enableHTTPSRequest
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&gotReq)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(httpsStatusResource{State: "failed", Domain: "1-2-3-4.sslip.io", Error: "Timeout during connect", Hint: "unreachable"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "https", "enable", "--email", "a@example.com", "--staging", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotReq.Email != "a@example.com" || !gotReq.Staging {
		t.Fatalf("request = %s %+v", gotMethod, gotReq)
	}
	if !strings.Contains(stdout, "state:      failed") || !strings.Contains(stdout, "port 80") {
		t.Errorf("stdout = %q, want failed state with the port 80 hint", stdout)
	}

	runCLIExpectOK(t, []string{"settings", "ingress", "https", "--api-url", srv.URL})
	if gotMethod != http.MethodGet {
		t.Errorf("default verb should be a status GET, got %s", gotMethod)
	}
}

func TestRun_SettingsIngressSet_FallbackDomains(t *testing.T) {
	var gotBody ingressSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(ingressSettingsResource{FallbackDomainsEnabled: true})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"settings", "ingress", "set", "--fallback-domains=false", "--api-url", srv.URL})
	if gotBody.FallbackDomainsEnabled {
		t.Error("--fallback-domains=false must send fallback_domains_enabled=false")
	}
}
