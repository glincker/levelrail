package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FirewallStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallStatusResource{
			Installed: true,
			Active:    true,
			Rules: []firewallRuleResource{
				{Port: 33001, Proto: "tcp", Owner: "app:web", Open: true},
				{Port: 54320, Proto: "tcp", Owner: "db:mydb", Open: false},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "status", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/system/firewall" {
		t.Errorf("request = %s %s, want GET /api/v1/system/firewall", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "app:web") || !strings.Contains(stdout, "33001") {
		t.Errorf("stdout = %q, want the app rule listed", stdout)
	}
	if !strings.Contains(stdout, "db:mydb") || !strings.Contains(stdout, "54320") {
		t.Errorf("stdout = %q, want the db rule listed", stdout)
	}
	if strings.Contains(stdout, "EXTRA") {
		t.Errorf("stdout = %q, want no EXTRA section when Extra is empty", stdout)
	}
}

func TestRun_FirewallStatus_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallStatusResource{
			Installed: true,
			Active:    true,
			Rules:     []firewallRuleResource{{Port: 33001, Proto: "tcp", Owner: "app:web", Open: true}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "status", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"owner": "app:web"`) {
		t.Errorf("stdout = %q, want the rule as JSON", stdout)
	}
}

func TestRun_FirewallStatus_ExtraRules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallStatusResource{
			Installed: true,
			Active:    true,
			Rules:     []firewallRuleResource{{Port: 33001, Proto: "tcp", Owner: "app:web", Open: true}},
			Extra:     []firewallRuleResource{{Port: 40000, Proto: "tcp", Owner: "app:deleted", Open: true}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "EXTRA (stale)") {
		t.Errorf("stdout = %q, want an EXTRA section", stdout)
	}
	if !strings.Contains(stdout, "app:deleted") {
		t.Errorf("stdout = %q, want the stale rule listed", stdout)
	}
}

func TestRun_FirewallStatus_NotInstalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallStatusResource{Installed: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "ufw is not installed on this control plane") {
		t.Errorf("stdout = %q, want the not-installed message", stdout)
	}
}

func TestRun_FirewallStatus_Inactive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallStatusResource{Installed: true, Active: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "ufw is installed but inactive") {
		t.Errorf("stdout = %q, want the inactive message", stdout)
	}
}

func TestRun_FirewallSync(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallSyncResource{
			FirewallStatusResource: firewallStatusResource{
				Installed: true,
				Active:    true,
				Rules:     []firewallRuleResource{{Port: 33001, Proto: "tcp", Owner: "app:web", Open: true}},
			},
			Applied: 1,
			Removed: 2,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "sync", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/system/firewall/sync" {
		t.Errorf("request = %s %s, want POST /api/v1/system/firewall/sync", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "applied: 1, removed: 2") {
		t.Errorf("stdout = %q, want applied/removed counts", stdout)
	}
}

func TestRun_FirewallSync_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallSyncResource{Applied: 1})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"firewall", "sync", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"applied": 1`) {
		t.Errorf("stdout = %q, want the sync result as JSON", stdout)
	}
}

func TestRun_FirewallSync_ErrorsExitNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(firewallSyncResource{
			Applied: 1,
			Errors:  []string{"ufw allow 33001/tcp: exit status 1"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"firewall", "sync", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (a partial failure), stdout=%q", got, exitAPIError, stdout.String())
	}
	if !strings.Contains(stdout.String(), "ufw allow 33001/tcp: exit status 1") {
		t.Errorf("stdout = %q, want the sync error listed", stdout.String())
	}
}

func TestRun_Firewall_NotConfigured(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotImplemented, `{"error":"firewall management is not configured on this control plane"}`)
	stderr := runCLIExpectAPIError(t, []string{"firewall", "status", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not configured") {
		t.Errorf("stderr = %q, want the server's not-configured message", stderr)
	}
}

func TestRun_Firewall_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"firewall", "-h"})
	if !strings.Contains(stdout, "firewall status") || !strings.Contains(stdout, "firewall sync") {
		t.Errorf("stdout = %q, want both subcommands listed", stdout)
	}
}

func TestRun_Firewall_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"firewall", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown firewall subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
