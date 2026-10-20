package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type upgradeRequest struct {
	method, path string
	body         map[string]any
}

func newUpgradeServer(t *testing.T) (*httptest.Server, func() []upgradeRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []upgradeRequest
	policy := `{"auto_upgrade":"off","window_cron":"0 3 * * 0","window_duration_seconds":7200,"window_timezone":"UTC","backup_before":true,"verify_after":true,"revert_on_failure":true,"notify":[],"inherited":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		seen = append(seen, upgradeRequest{r.Method, r.URL.Path, body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/databases/upgrade-summary":
			_, _ = io.WriteString(w, `{"items":[{"database":"main","engine":"postgres","version":"16.9","support":"supported","security":true,"available":3,"last_state":"reverted"}],"security_count":1,"eol_count":0}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/main/upgrades":
			_, _ = io.WriteString(w, `{"database":"main","engine":"postgres","version":"16.9","advice":{"engine":"postgres","current":"16.9","support":"supported","line":"16","advisories":["CVE-2025-8714"],"targets":[{"version":"16.11","kind":"patch","security":true,"advisories":["CVE-2025-8714"],"automatic":true}],"auto_max":"patch","catalog_updated":"2025-11-13"},"policy":`+policy+`,"blockers":["no backup target is configured"],"history":[{"id":"dbu_1","from_version":"16.4","to_version":"16.9","kind":"patch","source":"auto","state":"reverted","revert_path":"image","reason":"health check failed","timings":{}}]}`)
		case r.URL.Path == "/api/v1/settings/database-upgrades" || strings.HasSuffix(r.URL.Path, "/upgrade-policy"):
			_, _ = io.WriteString(w, policy)
		case strings.HasSuffix(r.URL.Path, "/upgrade-now"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"dbu_2","database_name":"main","from_version":"16.9","to_version":"16.11","state":"pending","timings":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []upgradeRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]upgradeRequest(nil), seen...)
	}
}

func TestRun_DatabasesUpgrades(t *testing.T) {
	srv, _ := newUpgradeServer(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"one database", []string{"databases", "upgrades", "main"}, []string{"16.11", "patch", "CVE-2025-8714", "blocked: no backup target", "reverted", "health check failed", "platform default"}},
		{"summary", []string{"databases", "upgrades"}, []string{"main", "postgres", "1 with security updates"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			for _, w := range tt.want {
				if !strings.Contains(stdout, w) {
					t.Errorf("stdout missing %q:\n%s", w, stdout)
				}
			}
		})
	}
}

func TestRun_DatabasesUpgradePolicy(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		check    func(t *testing.T, body map[string]any)
	}{
		{"overlays only set flags", []string{"databases", "upgrade-policy", "main", "--auto", "patch", "--timezone", "Europe/Berlin", "--revert=false", "--notify", "chn_a, chn_b"},
			"/api/v1/databases/main/upgrade-policy", func(t *testing.T, b map[string]any) {
				if b["auto_upgrade"] != "patch" || b["window_cron"] != "0 3 * * 0" || b["window_timezone"] != "Europe/Berlin" || b["revert_on_failure"] != false || b["verify_after"] != true {
					t.Errorf("body = %v", b)
				}
				if n, _ := b["notify"].([]any); len(n) != 2 {
					t.Errorf("notify = %v", b["notify"])
				}
			}},
		{"inherit", []string{"databases", "upgrade-policy", "main", "--inherit"}, "/api/v1/databases/main/upgrade-policy", func(t *testing.T, b map[string]any) {
			if b["inherit"] != true {
				t.Errorf("body = %v", b)
			}
		}},
		{"platform default", []string{"databases", "upgrade-policy", "--platform", "--auto", "minor", "--duration", "90m"}, "/api/v1/settings/database-upgrades", func(t *testing.T, b map[string]any) {
			if b["auto_upgrade"] != "minor" || b["window_duration_seconds"] != float64(5400) {
				t.Errorf("body = %v", b)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, seen := newUpgradeServer(t)
			runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			var put *upgradeRequest
			for _, r := range seen() {
				if r.method == http.MethodPut {
					r := r
					put = &r
				}
			}
			if put == nil || put.path != tt.wantPath {
				t.Fatalf("PUT = %+v, want %s", put, tt.wantPath)
			}
			tt.check(t, put.body)
		})
	}
}

func TestRun_DatabasesUpgradeNow(t *testing.T) {
	srv, seen := newUpgradeServer(t)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "upgrade-now", "main", "16.11", "--confirm", "main", "--api-url", srv.URL})
	if !strings.Contains(stdout, "dbu_2") {
		t.Errorf("stdout = %q", stdout)
	}
	reqs := seen()
	if len(reqs) != 1 || reqs[0].body["version"] != "16.11" || reqs[0].body["confirm"] != "main" {
		t.Errorf("requests = %+v", reqs)
	}

	var out, errOut strings.Builder
	if code := run("levelrail-cli-test", []string{"databases", "upgrade-now", "main", "16.11", "--confirm", "other", "--api-url", srv.URL}, &out, &errOut, envMap()); code == 0 {
		t.Error("a mismatched --confirm succeeded")
	}
	if code := run("levelrail-cli-test", []string{"databases", "upgrade-now", "main"}, &out, &errOut, envMap()); code != exitUsage {
		t.Errorf("missing version exit = %d, want %d", code, exitUsage)
	}
}
