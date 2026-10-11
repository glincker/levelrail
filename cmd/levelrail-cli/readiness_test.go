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

func TestRun_Readiness(t *testing.T) {
	tests := []struct {
		name     string
		report   apiclient.ServerReadiness
		wantCode int
		wantOut  []string
	}{
		{
			name: "behind an existing proxy",
			report: apiclient.ServerReadiness{
				Mode: "behind_proxy", Proxy: "traefik", Summary: "Ports 80 and 443 are taken by traefik.",
				NextStep: "curl -fsSL x | sudo sh -s -- --coexist",
				Holders:  map[int]string{80: "container coolify-proxy (image traefik:v3)"},
				Checks:   []apiclient.ReadinessCheck{{ID: "port_80", Name: "Port 80", Status: "warn", Detail: "held by traefik", Fix: "install behind it"}},
			},
			wantCode: exitOK,
			wantOut:  []string{"[warn] Port 80", "fix: install behind it", "port 80 is held by container coolify-proxy", "recommended mode: behind_proxy", "--coexist"},
		},
		{
			name: "blocked exits non zero",
			report: apiclient.ServerReadiness{
				Mode: "own_ports", Blocked: true, NextStep: "upgrade docker",
				Checks: []apiclient.ReadinessCheck{{ID: "docker", Name: "Docker Engine", Status: "fail", Detail: "too old"}},
			},
			wantCode: exitCheckFailed,
			wantOut:  []string{"[fail] Docker Engine"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotDomain string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotDomain = r.URL.Query().Get("domain")
				_ = json.NewEncoder(w).Encode(tt.report)
			}))
			defer srv.Close()
			var out, errOut bytes.Buffer
			code := run("levelrail-cli-test", []string{"readiness", "--domain", "app.example.com", "--api-url", srv.URL}, &out, &errOut, envMap())
			if code != tt.wantCode {
				t.Fatalf("exit %d, want %d (stderr %s)", code, tt.wantCode, errOut.String())
			}
			if gotDomain != "app.example.com" {
				t.Errorf("domain sent = %q", gotDomain)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}
