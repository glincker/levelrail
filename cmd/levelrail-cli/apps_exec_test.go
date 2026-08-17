package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppsExec_RealExitCode is the requirement this package's task
// singles out: run()'s returned int (what main() would hand to
// os.Exit) must be the remote command's own real exit code, not this
// CLI's usual exitOK/exitAPIError enum, whenever the exec mechanism
// itself succeeded (a 200 from the server, per internal/api/exec.go's
// own doc comment on execResponse.ExitCode: "always the command's real
// exit code... a nonzero ExitCode here means the exec mechanism itself
// worked fine and the command it ran chose to fail").
func TestRun_AppsExec_RealExitCode(t *testing.T) {
	tests := []struct {
		name         string
		serverExit   int
		serverStdout string
	}{
		{name: "success", serverExit: 0, serverStdout: "ok\n"},
		{name: "nonzero exit code passed through", serverExit: 3, serverStdout: ""},
		{name: "large exit code passed through", serverExit: 137, serverStdout: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/exec" {
					t.Errorf("request = %s %s, want POST /api/v1/apps/web/exec", r.Method, r.URL.Path)
				}
				var req execRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				if req.Command != "sh" || len(req.Args) != 2 || req.Args[0] != "-c" {
					t.Errorf("request = %+v, want command sh -c <script>", req)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(execResponse{Stdout: tt.serverStdout, ExitCode: tt.serverExit})
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "exec", "web", "--api-url", srv.URL, "--", "sh", "-c", "exit"}, &stdout, &stderr, envMap())
			if got != tt.serverExit {
				t.Fatalf("exit = %d, want %d (the server's own exit_code) (stdout=%q stderr=%q)", got, tt.serverExit, stdout.String(), stderr.String())
			}
			if stdout.String() != tt.serverStdout {
				t.Errorf("stdout = %q, want the remote command's real stdout %q", stdout.String(), tt.serverStdout)
			}
		})
	}
}

func TestRun_AppsExec_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(execResponse{Stdout: "hi\n", ExitCode: 5})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec", "web", "--api-url", srv.URL, "--json", "--", "echo", "hi"}, &stdout, &stderr, envMap())
	if got != 5 {
		t.Fatalf("exit = %d, want 5 (the server's exit_code) even in --json mode", got)
	}
	if !strings.Contains(stdout.String(), `"exit_code": 5`) {
		t.Errorf("stdout = %q, want the full exec result as JSON", stdout.String())
	}
}

func TestRun_AppsExec_Stderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(execResponse{Stderr: "boom\n", ExitCode: 1})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec", "web", "--api-url", srv.URL, "--", "false"}, &stdout, &stderr, envMap())
	if got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want the remote command's real stderr", stderr.String())
	}
}

func TestRun_AppsExec_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"app has no running container"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec", "web", "--api-url", srv.URL, "--", "true"}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (a genuine API error, no remote exit code was ever learned)", got, exitAPIError)
	}
	if !strings.Contains(stderr.String(), "app has no running container") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsExec_MissingCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an app name and a command") {
		t.Errorf("stderr = %q, want a missing-command usage error", stderr.String())
	}
}

func TestRun_AppsExec_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsExec_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "exec", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps exec") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
