package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDeployFromCache(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCommit string
	}{
		{"defaults to the last good commit", []string{"apps", "deploy", "web", "--from-cache"}, ""},
		{"explicit commit", []string{"apps", "deploy", "web", "--from-cache", "--commit", "deadbeef"}, "deadbeef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/apps/web/deploys/from-cache" || r.Method != http.MethodPost {
					http.NotFound(w, r)
					return
				}
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(buildTriggerResponse{ID: "dep_9"})
			}))
			t.Cleanup(srv.Close)

			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if c, _ := got["commit"].(string); c != tt.wantCommit {
				t.Errorf("commit = %q, want %q", c, tt.wantCommit)
			}
			if !strings.Contains(stdout, "dep_9") || !strings.Contains(stdout, "source cache") {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}

func TestRun_AppsDeployFromCacheRejectsMixedFlags(t *testing.T) {
	var out, errOut strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "deploy", "web", "--from-cache", "--latest", "--api-url", "http://127.0.0.1:1"}, &out, &errOut, envMap())
	if code == 0 {
		t.Fatalf("exit = 0, want a usage error; stderr = %s", errOut.String())
	}
}
