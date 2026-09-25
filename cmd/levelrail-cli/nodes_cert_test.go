package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_NodesReenrollToken(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.NodeReenrollTokenResponse{ //nolint:gosec // test fixture value, not a real credential
			Token: "reenroll-value", NodeID: "nd_1", ExpiresAt: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC),
			CAFingerprint: "abcd", AgentBinary: "brand-agent",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "reenroll-token", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/reenroll-token" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{"shown once", "APP_REENROLL_TOKEN=reenroll-value", "APP_CA_FINGERPRINT=abcd", "./brand-agent reenroll"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
}

func TestRun_NodesRevokeCert(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.NodeResource{ID: "nd_1", Cert: &apiclient.NodeCertResource{State: "revoked"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "revoke-cert", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/revoke-cert" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "revoked") || !strings.Contains(stdout, "reenroll-token nd_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestNodeCertAndAgentColumns(t *testing.T) {
	days := 12
	tests := []struct {
		name  string
		cert  *apiclient.NodeCertResource
		agent *apiclient.NodeAgentResource
		want  []string
	}{
		{"expiring with version", &apiclient.NodeCertResource{State: "expiring", DaysRemaining: &days}, &apiclient.NodeAgentResource{Version: "v1.0.0"}, []string{"expiring 12d", "v1.0.0"}},
		{"expired and outdated", &apiclient.NodeCertResource{State: "expired", DaysRemaining: &days}, &apiclient.NodeAgentResource{Version: "v0.1.0", Outdated: true}, []string{"expired", "v0.1.0 (outdated)"}},
		{"older control plane", nil, nil, []string{"-", "-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			printNodesTable(&b, []nodeResource{{ID: "nd_1", Name: "a", Cert: tt.cert, Agent: tt.agent}})
			out := b.String()
			if !strings.Contains(out, "CERT") || !strings.Contains(out, "AGENT") {
				t.Fatalf("table header missing CERT/AGENT: %q", out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("table %q does not contain %q", out, w)
				}
			}
		})
	}
}
