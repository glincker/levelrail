package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsMaintenance(t *testing.T) {
	tests := []struct {
		verb, wantMethod, wantOut string
		extra                     []string
	}{
		{"show", http.MethodGet, "maintenance: 1 of 1 domains", nil},
		{"set", http.MethodPut, "maintenance: 1 of 1 domains", []string{"--for", "2h", "--title", "Back soon"}},
		{"clear", http.MethodDelete, "disabled for every domain", nil},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				_ = json.NewEncoder(w).Encode(apiclient.AppMaintenanceResource{App: "web", Enabled: true, All: true, Total: 1, Active: 1,
					Domains: []apiclient.AppMaintenanceDomain{{DomainMaintenanceResource: apiclient.DomainMaintenanceResource{Domain: "a.example.com", Enabled: true}}}})
			}))
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			args := append([]string{"apps", "maintenance", tt.verb, "web", "--api-url", srv.URL}, tt.extra...)
			got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if got != exitOK || gotMethod != tt.wantMethod || gotPath != "/api/v1/apps/web/maintenance" || !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("exit=%d %s %s stdout=%q stderr=%q", got, gotMethod, gotPath, stdout.String(), stderr.String())
			}
			if tt.verb == "set" && !strings.Contains(gotBody, "Back soon") {
				t.Fatalf("body = %q", gotBody)
			}
		})
	}
}

func TestRun_AppsMaintenanceUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"apps", "maintenance"}, {"apps", "maintenance", "show"}, {"apps", "maintenance", "set", "web", "--for", "1h", "--until", "2030-01-01T00:00:00Z"}} {
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitUsage {
			t.Fatalf("%v: exit = %d", args, got)
		}
	}
}
