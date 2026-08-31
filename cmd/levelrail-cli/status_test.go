package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Status_DockerConnected(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemStatusResource{
			DockerConnected:     true,
			SecretsConfigured:   true,
			TelemetryConfigured: false,
			AlertsConfigured:    true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"status", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/system/status" {
		t.Errorf("request = %s %s, want GET /api/v1/system/status", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "docker:              connected") {
		t.Errorf("stdout = %q, want docker connected line", stdout)
	}
}

func TestRun_Status_DockerUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemStatusResource{
			DockerConnected: false,
			DockerError:     "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "UNREACHABLE") || !strings.Contains(stdout, "Is the docker daemon running?") {
		t.Errorf("stdout = %q, want the UNREACHABLE line with the daemon error", stdout)
	}
}

func TestRun_Status_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemStatusResource{DockerConnected: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"status", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"docker_connected": true`) {
		t.Errorf("stdout = %q, want the status as JSON", stdout)
	}
}

func TestRun_Status_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"status", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_Status_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"status", "-h"})
	if !strings.Contains(stderr, "status") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
