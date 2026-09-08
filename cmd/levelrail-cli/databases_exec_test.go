package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_DatabasesExec_RealExitCode mirrors
// TestRun_AppsExec_RealExitCode: run()'s returned int must be the
// remote command's own real exit code whenever the exec mechanism
// itself succeeded.
func TestRun_DatabasesExec_RealExitCode(t *testing.T) {
	tests := []struct {
		name         string
		serverExit   int
		serverStdout string
	}{
		{name: "success", serverExit: 0, serverStdout: "PONG\n"},
		{name: "nonzero exit code passed through", serverExit: 2, serverStdout: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/databases/main/exec" {
					t.Errorf("request = %s %s, want POST /api/v1/databases/main/exec", r.Method, r.URL.Path)
				}
				var req execRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				if req.Command != "redis-cli" || len(req.Args) != 1 || req.Args[0] != "ping" {
					t.Errorf("request = %+v, want command redis-cli ping", req)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(execResponse{Stdout: tt.serverStdout, ExitCode: tt.serverExit})
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"databases", "exec", "main", "--api-url", srv.URL, "--", "redis-cli", "ping"}, &stdout, &stderr, envMap())
			if got != tt.serverExit {
				t.Fatalf("exit = %d, want %d (the server's own exit_code) (stdout=%q stderr=%q)", got, tt.serverExit, stdout.String(), stderr.String())
			}
			if stdout.String() != tt.serverStdout {
				t.Errorf("stdout = %q, want the remote command's real stdout %q", stdout.String(), tt.serverStdout)
			}
		})
	}
}

func TestRun_DatabasesExec_Stderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(execResponse{Stderr: "connection refused\n", ExitCode: 1})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "exec", "main", "--api-url", srv.URL, "--", "redis-cli", "ping"}, &stdout, &stderr, envMap())
	if got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "connection refused") {
		t.Errorf("stderr = %q, want the remote command's real stderr", stderr.String())
	}
}

func TestRun_DatabasesExec_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"database has no running container"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "exec", "main", "--api-url", srv.URL, "--", "redis-cli", "ping"}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (a genuine API error, no remote exit code was ever learned)", got, exitAPIError)
	}
	if !strings.Contains(stderr.String(), "database has no running container") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_DatabasesExec_MissingCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "exec", "main"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires a database name and a command") {
		t.Errorf("stderr = %q, want a missing-command usage error", stderr.String())
	}
}

func TestRun_DatabasesExec_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "exec"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_DatabasesExec_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "exec", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "databases exec") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
