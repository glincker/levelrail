package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsDeploysAttempts_FailOnError(t *testing.T) {
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		newest   string
		args     []string
		wantExit int
	}{
		{"latest failed with flag", "failed", []string{"--fail-on-error"}, exitDeployNotHealthy},
		{"latest failed without flag", "failed", nil, exitOK},
		{"latest succeeded with flag", "succeeded", []string{"--fail-on-error"}, exitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]deployAttemptResource{
					{ID: "dep_2", ServiceName: "web", Image: "levelrail/web:2", Source: "image", Status: tc.newest, StartedAt: started},
					{ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1", Source: "image", Status: "succeeded", StartedAt: started},
				})
			}))
			defer srv.Close()

			args := append([]string{"apps", "deploys", "attempts", "web", "--api-url", srv.URL}, tc.args...)
			var out, errb bytes.Buffer
			got := run("levelrail-cli-test", args, &out, &errb, envMap())
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantExit, errb.String())
			}
			if !strings.Contains(out.String(), "dep_2") || !strings.Contains(out.String(), "dep_1") {
				t.Errorf("stdout = %q, want both attempts in the table", out.String())
			}
		})
	}
}
