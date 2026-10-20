package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_NodesSecurity(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	days := 40
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/security") {
			_ = json.NewEncoder(w).Encode(apiclient.NodeTrust{
				NodeID: "nd_1", Status: apiclient.NodeTrustStatus{State: "verified"},
				CA:         &apiclient.NodeTrustCA{Fingerprint: "abcdef", PinState: "pinned"},
				Cert:       &apiclient.NodeTrustCert{Fingerprint: "0102", Serial: "2a", NotAfter: &now, DaysRemaining: &days, KeyOrigin: "agent"},
				Renewal:    &apiclient.NodeTrustRenewal{NextRenewAt: &now, NextRenewEstimated: true, RenewalCount: 2},
				Validation: &apiclient.NodeTrustValidation{WindowHours: 24, Accepted: 5, Rejected: 1},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]apiclient.NodeSecurityEvent{{Kind: "connect", Outcome: "rejected", Reason: "expired", SourceIP: "203.0.113.5", Count: 4, LastAt: now}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "security", "nd_1", "--rejected", "--api-url", srv.URL})
	if paths[1] != "/api/v1/nodes/nd_1/security/events?limit=20&outcome=rejected" {
		t.Errorf("events request = %q", paths[1])
	}
	for _, want := range []string{
		"connection verified: mutual TLS, certificate matches this node", "5 accepted, 1 rejected",
		"AB:CD:EF", "pinned", "01:02", "(estimated)", "2 renewals", "expired", "203.0.113.5",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_NodesSecurityLocalNodeHasNoCertificate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.NodeTrust{NodeID: "nd_local", IsLocal: true, Status: apiclient.NodeTrustStatus{State: "local"}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "security", "nd_local", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no agent certificate is used") || strings.Contains(stdout, "fingerprint") {
		t.Errorf("stdout = %q, want the plain in-process explanation and no certificate fields", stdout)
	}
}

func TestRun_NodesCertRenew(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"requested":true}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "cert", "renew", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/cert/renew" || !strings.Contains(stdout, "renewal requested") {
		t.Errorf("request = %s %s, stdout = %q", gotMethod, gotPath, stdout)
	}
}
