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

func TestRun_NetworkMesh(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantExit  int
		wantOut   []string
		wantReq   string
		wantForce bool
	}{
		{name: "status", args: []string{"network", "mesh", "status"}, wantOut: []string{"mesh:        off", "fix: sudo modprobe tun"}, wantReq: "GET /api/v1/mesh/settings"},
		{name: "enable", args: []string{"network", "mesh", "enable"}, wantReq: "PUT /api/v1/mesh/settings"},
		{name: "disable force", args: []string{"network", "mesh", "disable", "--force"}, wantReq: "PUT /api/v1/mesh/settings", wantForce: true},
		{name: "unknown", args: []string{"network", "mesh", "frobnicate"}, wantExit: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqs []string
			var body apiclient.UpdateMeshSettingsRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqs = append(reqs, r.Method+" "+r.URL.Path)
				if r.Method == http.MethodPut {
					_ = json.NewDecoder(r.Body).Decode(&body)
				}
				_ = json.NewEncoder(w).Encode(apiclient.MeshSettings{
					Source: "default",
					Checks: []apiclient.MeshCheck{{ID: "wireguard", Status: "fail", Detail: "no tun", Hint: "sudo modprobe tun"}},
				})
			}))
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
			if body.Force != tt.wantForce {
				t.Errorf("force = %v, want %v", body.Force, tt.wantForce)
			}
		})
	}
}
