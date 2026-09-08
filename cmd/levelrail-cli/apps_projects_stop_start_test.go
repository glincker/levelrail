package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppsProjectsStartStop covers "apps projects start" and "apps
// projects stop", table-driven since the two commands are structurally
// identical (same request shape, same success/error paths), differing
// only in verb and URL suffix, mirroring TestRun_AppsStartStop's own
// shape for the single-app counterpart.
func TestRun_AppsProjectsStartStop(t *testing.T) {
	tests := []struct {
		action string
		path   string
	}{
		{action: "start", path: "/api/v1/projects/proj_1/start"},
		{action: "stop", path: "/api/v1/projects/proj_1/stop"},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(projectLifecycleResult{
					SucceededApps:      []string{"web"},
					SucceededDatabases: []string{"main"},
				})
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "projects", tt.action, "proj_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != exitOK {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
			}
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want POST", gotMethod)
			}
			if gotPath != tt.path {
				t.Errorf("path = %q, want %q", gotPath, tt.path)
			}
			if !strings.Contains(stdout.String(), "1 app(s) and 1 database(s)") {
				t.Errorf("stdout = %q, want the per-resource-type count", stdout.String())
			}
		})
	}
}

func TestRun_AppsProjectsStartStop_ReportsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(projectLifecycleResult{
			SucceededApps:   []string{"web"},
			FailedApps:      []string{"worker"},
			FailedDatabases: []string{"main"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "stop", "proj_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "failed apps: worker") {
		t.Errorf("stdout = %q, want the failed app listed", stdout)
	}
	if !strings.Contains(stdout, "failed databases: main") {
		t.Errorf("stdout = %q, want the failed database listed", stdout)
	}
}

func TestRun_AppsProjectsStartStop_NotFound(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"project not found"}`)

			stderr := runCLIExpectAPIError(t, []string{"apps", "projects", action, "ghost", "--api-url", srv.URL})
			if !strings.Contains(stderr, "project not found") {
				t.Errorf("stderr = %q, want the server's error message", stderr)
			}
		})
	}
}

func TestRun_AppsProjectsStartStop_NoID(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "projects", action}, &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "requires exactly one") {
				t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
			}
		})
	}
}

func TestRun_AppsProjectsStartStop_Help(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			_, stderr := runCLIExpectOK(t, []string{"apps", "projects", action, "-h"})
			if !strings.Contains(stderr, "apps projects "+action) {
				t.Errorf("stderr = %q, want usage text", stderr)
			}
		})
	}
}
