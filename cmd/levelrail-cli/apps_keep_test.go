package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRun_AppsKeep(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		respond  map[string]any
		wantOut  string
		wantJSON bool
	}{
		{
			name:    "kept",
			args:    []string{"apps", "keep", "ntfy-a"},
			respond: map[string]any{"name": "ntfy-a", "kept": []string{"ntfy-a"}, "kept_at": "2026-10-14T12:00:00Z"},
			wantOut: `app "ntfy-a" is now a permanent app (ntfy-a)`,
		},
		{
			name:    "already kept",
			args:    []string{"apps", "keep", "ntfy-a"},
			respond: map[string]any{"name": "ntfy-a", "kept": []string{}, "kept_at": "2026-10-14T12:00:00Z", "already_kept": true},
			wantOut: "already kept",
		},
		{
			name:     "json",
			args:     []string{"apps", "keep", "ntfy-a", "--json"},
			respond:  map[string]any{"name": "ntfy-a", "kept": []string{"ntfy-a"}, "kept_at": "2026-10-14T12:00:00Z"},
			wantOut:  `"already_kept": false`,
			wantJSON: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := runStartStopSuccess(t, tt.args, "/api/v1/apps/ntfy-a/keep", func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(tt.respond)
			})
			if !strings.Contains(stdout, tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.wantOut)
			}
		})
	}
}

func TestRun_AppsKeep_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "not a trial", status: http.StatusConflict, body: `{"error":"app is not a trial deployment"}`, want: "not a trial"},
		{name: "missing", status: http.StatusNotFound, body: `{"error":"app not found"}`, want: "app not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newJSONErrorServer(t, tt.status, tt.body)
			stderr := runCLIExpectAPIError(t, []string{"apps", "keep", "web", "--api-url", srv.URL})
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestRun_AppsKeep_Usage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"apps", "keep"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	_, help := runCLIExpectOK(t, []string{"apps", "keep", "-h"})
	if !strings.Contains(help, "apps keep <name>") {
		t.Errorf("help = %q", help)
	}
}
