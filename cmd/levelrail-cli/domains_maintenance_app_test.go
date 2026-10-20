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

func TestRun_DomainsMaintenanceAppWide(t *testing.T) {
	tests := []struct {
		verb       string
		wantMethod string
		wantOut    string
	}{
		{"set", http.MethodPut, "maintenance: 2 of 2 domains"},
		{"get", http.MethodGet, "alerts: silenced (sil_1)"},
		{"clear", http.MethodDelete, "disabled for every domain"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			var gotMethod, gotPath string
			d := apiclient.DomainMaintenanceResource{Domain: "a.example.com", Enabled: true, Ongoing: true, AlertsSilenced: true, SilenceID: "sil_1"}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				_ = json.NewEncoder(w).Encode(apiclient.AppMaintenanceResource{App: "web", Enabled: true, All: true, Total: 2, Active: 2,
					Domains: []apiclient.AppMaintenanceDomain{{DomainMaintenanceResource: d}}})
			}))
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"domains", "maintenance", tt.verb, "--app", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != exitOK || gotMethod != tt.wantMethod || gotPath != "/api/v1/apps/web/maintenance" || !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("exit=%d %s %s stdout=%q stderr=%q", got, gotMethod, gotPath, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRun_DomainsMaintenanceStatusShowsEffects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.FleetMaintenanceResource{Items: []apiclient.FleetMaintenanceEntry{{
			App: "web", DomainMaintenanceResource: apiclient.DomainMaintenanceResource{Domain: "a.example.com", Enabled: true, Ongoing: true, AlertsSilenced: true, SilenceID: "sil_1"},
		}}})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "maintenance", "status", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d (%s)", got, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "alerts: silenced") || !strings.Contains(out, "status page: listed as ongoing") {
		t.Fatalf("stdout = %q", out)
	}
}
