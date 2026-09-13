package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestRun_AppsStartStop covers "apps start" and "apps stop", table-driven
// since the two commands are structurally identical (same request shape,
// same success/error paths), differing only in verb and URL suffix.
func TestRun_AppsStartStop(t *testing.T) {
	tests := []struct {
		action string
		path   string
	}{
		{action: "start", path: "/api/v1/apps/web/start"},
		{action: "stop", path: "/api/v1/apps/web/stop"},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			stdout := runStartStopSuccess(t, []string{"apps", tt.action, "web", "--json"}, tt.path, func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "levelrail/web:1", Port: 3000})
			})
			if !strings.Contains(stdout, `"name": "web"`) {
				t.Errorf("stdout = %q, want it to contain the app as JSON", stdout)
			}
		})
	}
}

func TestRun_AppsStartStop_NotFound(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

			stderr := runCLIExpectAPIError(t, []string{"apps", action, "ghost", "--api-url", srv.URL})
			if !strings.Contains(stderr, "app not found") {
				t.Errorf("stderr = %q, want the server's error message", stderr)
			}
		})
	}
}

func TestRun_AppsStartStop_NoName(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", action}, &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "requires exactly one") {
				t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
			}
		})
	}
}

func TestRun_AppsStartStop_Help(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			_, stderr := runCLIExpectOK(t, []string{"apps", action, "-h"})
			if !strings.Contains(stderr, "apps "+action) {
				t.Errorf("stderr = %q, want usage text", stderr)
			}
		})
	}
}
