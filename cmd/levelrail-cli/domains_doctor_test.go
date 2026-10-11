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

func TestRun_DomainsDoctor_PrioritisedFixes(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewEncoder(w).Encode(apiclient.DomainDoctorReport{
			Domain: "shop.example.com", Status: "problems", Probed: true,
			Checks: []apiclient.DoctorCheck{
				{ID: "redirect.chain", Title: "HTTP does not redirect to HTTPS", State: "warn", Tier: 4, Fix: &apiclient.DoctorFix{Summary: "Turn on Force HTTPS."}},
				{ID: "dns.resolves", Title: "DNS resolves", State: "pass", Tier: 1},
				{ID: "tls.certificate", Title: "A default certificate from Traefik is served", State: "fail", Tier: 2,
					Fix: &apiclient.DoctorFix{Summary: "Give that proxy a route.", Action: &apiclient.DoctorAction{Kind: "request", Label: "Show proxy setup", API: "GET /api/v1/system/reverse-proxy"}}},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "doctor", "shop", "shop.example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/shop/domains/shop.example.com/doctor" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	traefik := strings.Index(stdout, "1. [fail] A default certificate from Traefik")
	redirect := strings.Index(stdout, "2. [warn] HTTP does not redirect")
	if traefik < 0 || redirect < 0 {
		t.Fatalf("stdout = %q, want failures listed before warnings", stdout)
	}
	if !strings.Contains(stdout, "Show proxy setup: GET /api/v1/system/reverse-proxy") || !strings.Contains(stdout, "2 to fix, 1 passed") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DomainsDoctor_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "doctor", "shop"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_DomainsSummary(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, apiclient.TrafficSummary{Attention: 2,
		Domains:      apiclient.TrafficDomainCounts{Total: 5, Live: 3, NeedsAttention: 2},
		Certificates: apiclient.TrafficCertCounts{WindowDays: 30, Expiring: 1}})
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "summary", "--api-url", srv.URL})
	if gotPath != "/api/v1/traffic/summary" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(stdout, "needs attention  2") || !strings.Contains(stdout, "1 expiring within 30 days") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DomainsActivity(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(apiclient.ActivityPage{NextCursor: "2026-10-01T00:00:00.000000000Z", Events: []apiclient.ActivityEvent{
			{ID: "e1", Kind: "certificate", Title: "Certificate issued", Actor: apiclient.ActivityActor{Type: "system", Name: "Automatic TLS"}},
		}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "activity", "shop.example.com", "--limit", "5", "--actions", "dns_record.", "--api-url", srv.URL})
	if gotURL != "/api/v1/domains/shop.example.com/activity?actions=dns_record.&limit=5" {
		t.Errorf("url = %q", gotURL)
	}
	if !strings.Contains(stdout, "Certificate issued") || !strings.Contains(stdout, "--before 2026-10-01") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AuditLogResourceFilter(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"audit-log", "--resource", "domain:shop.example.com", "--actions", "dns_record.,domain.", "--api-url", srv.URL})
	if !strings.Contains(gotURL, "resource=domain%3Ashop.example.com") || !strings.Contains(gotURL, "actions=dns_record.%2Cdomain.") {
		t.Errorf("url = %q", gotURL)
	}
}

func TestTrafficAlertCondition(t *testing.T) {
	cases := []struct {
		r    alertRuleResource
		want string
	}{
		{alertRuleResource{Kind: "cert_expiring"}, "within 14 days"},
		{alertRuleResource{Kind: "cert_expiring", Threshold: 7}, "within 7 days"},
		{alertRuleResource{Kind: "cert_renewal_stalled"}, "stalled"},
		{alertRuleResource{Kind: "domain_not_resolving", ForDuration: "1h"}, "for 1h"},
		{alertRuleResource{Kind: "nope"}, "-"},
	}
	for _, tc := range cases {
		if got := alertRuleCondition(tc.r); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q does not contain %q", tc.r.Kind, got, tc.want)
		}
	}
}
