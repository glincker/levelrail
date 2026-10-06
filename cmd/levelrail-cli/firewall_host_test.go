package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FirewallHost_StatusEnableDryRunAndDisable(t *testing.T) {
	var gotCalls []string
	var gotDryRun bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCalls = append(gotCalls, r.Method+" "+r.URL.Path)
		var body struct {
			DryRun bool `json:"dry_run"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotDryRun = body.DryRun
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hostFirewallResource{
			Installed: true,
			Required:  []apiclientHostFirewallPort{{Port: 22, Protocol: "tcp"}, {Port: 443, Protocol: "udp"}},
			Commands:  []string{"ufw allow 22/tcp", "ufw --force enable"},
		})
	}))
	defer srv.Close()

	run1 := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v exit = %d (stderr=%q)", args, got, stderr.String())
		}
		return stdout.String()
	}

	if out := run1("firewall", "status"); !strings.Contains(out, "22/tcp") || !strings.Contains(out, "443/udp") {
		t.Errorf("status output = %q, want required ports listed", out)
	}
	if out := run1("firewall", "enable", "--dry-run"); !strings.Contains(out, "ufw allow 22/tcp") || !gotDryRun {
		t.Errorf("enable --dry-run output = %q, dry_run sent = %v, want commands shown and dry_run true", out, gotDryRun)
	}
	run1("firewall", "disable")

	want := []string{"GET /api/v1/firewall/host", "POST /api/v1/firewall/host/enable", "POST /api/v1/firewall/host/disable"}
	if strings.Join(gotCalls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", gotCalls, want)
	}
}
