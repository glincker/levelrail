package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppsExecAccess_EnableAndDisable exercises "apps exec-access
// enable/disable <name>" end to end against a fake control plane,
// mirroring apps_rollback_test.go's own shape for the sibling
// auto-rollback commands this file has no dedicated test for.
func TestRun_AppsExecAccess_EnableAndDisable(t *testing.T) {
	tests := []struct {
		verb        string
		wantEnabled bool
	}{
		{verb: "enable", wantEnabled: true},
		{verb: "disable", wantEnabled: false},
	}
	for _, tc := range tests {
		t.Run(tc.verb, func(t *testing.T) {
			var gotMethod, gotPath string
			var gotBody struct {
				Enabled bool `json:"enabled"`
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]bool{"enabled": tc.wantEnabled})
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "exec-access", tc.verb, "web", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
			if got != exitOK {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
			}
			if gotMethod != http.MethodPut {
				t.Errorf("method = %q, want PUT", gotMethod)
			}
			if gotPath != "/api/v1/apps/web/exec-access" {
				t.Errorf("path = %q, want /api/v1/apps/web/exec-access", gotPath)
			}
			if gotBody.Enabled != tc.wantEnabled {
				t.Errorf("request body Enabled = %v, want %v", gotBody.Enabled, tc.wantEnabled)
			}
			if !strings.Contains(stdout.String(), `"enabled": `) {
				t.Errorf("stdout = %q, want the resulting setting as JSON", stdout.String())
			}
		})
	}
}

func TestRun_AppsExecAccess_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/exec-access" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"enabled": true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "exec-access", "status", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "exec access: true") {
		t.Errorf("stdout = %q, want the human status line", stdout)
	}
}

func TestRun_AppsExecAccess_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec-access", "status"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsExecAccess_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec-access", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps exec-access subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsExecAccess_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec-access", "enable", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "forbidden") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsExecAccess_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec-access", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps exec-access") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}
