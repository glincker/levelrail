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

func dockerGuardServer(t *testing.T, gotReqs *[]string, gotBody *apiclient.UpdateDockerGuardRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotReqs = append(*gotReqs, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		res := apiclient.DockerGuardResource{
			Mode: "audit", Source: "default", Effective: "audit", Running: true, Socket: "/data/guard/docker-guard.sock",
			Configured: true, WindowSeconds: 7 * 86400, WouldDenyTotal: 3,
			Window: []apiclient.DockerGuardRuleSummary{{Rule: "privileged", WouldDeny: 3, LastPath: "POST docker:/containers/create"}},
		}
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(gotBody)
			res.Mode, res.Effective, res.Source = gotBody.Mode, gotBody.Mode, "settings"
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
}

func TestRun_DockerGuard(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantOut  []string
		wantReq  string
		wantMode string
	}{
		{name: "status", args: []string{"docker-guard", "status"}, wantOut: []string{"mode:        audit (from default)", "privileged", "3 would deny"}, wantReq: "GET /api/v1/system/docker-guard"},
		{name: "set enforce", args: []string{"docker-guard", "set", "--mode", "enforce"}, wantOut: []string{"mode:        enforce (from settings)"}, wantReq: "PUT /api/v1/system/docker-guard", wantMode: "enforce"},
		{name: "set rejects unknown mode", args: []string{"docker-guard", "set", "--mode", "loose"}, wantExit: exitUsage},
		{name: "set needs mode", args: []string{"docker-guard", "set"}, wantExit: exitUsage},
		{name: "unknown subcommand", args: []string{"docker-guard", "frobnicate"}, wantExit: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqs []string
			var body apiclient.UpdateDockerGuardRequest
			srv := dockerGuardServer(t, &reqs, &body)
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append(tt.args, "--api-url", srv.URL), &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d; stderr %s", got, tt.wantExit, stderr.String())
			}
			for _, w := range tt.wantOut {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout missing %q:\n%s", w, stdout.String())
				}
			}
			if tt.wantReq != "" && (len(reqs) == 0 || reqs[len(reqs)-1] != tt.wantReq) {
				t.Errorf("requests = %v, want last %q", reqs, tt.wantReq)
			}
			if tt.wantExit != exitOK && len(reqs) != 0 {
				t.Errorf("usage error still made requests: %v", reqs)
			}
			if body.Mode != tt.wantMode {
				t.Errorf("PUT mode = %q, want %q", body.Mode, tt.wantMode)
			}
		})
	}
}
