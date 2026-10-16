package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsBuildStatsAndLimits(t *testing.T) {
	var putTimeout int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/web/build-stats":
			_ = json.NewEncoder(w).Encode(apiclient.BuildStats{App: "web", Limit: 20,
				Trend: apiclient.BuildTrend{Builds: 2, Succeeded: 1, Failed: 1, P50Seconds: 12, P95Seconds: 12, MinSeconds: 12, MaxSeconds: 12, CacheKnown: true, CacheHitRatio: 0.75},
				Runs:  []apiclient.BuildRun{{StartedAt: time.Unix(0, 0), Status: "success", DurationSeconds: 12, CachedSteps: 3, TotalSteps: 4}}})
		case r.URL.Path == "/api/v1/apps/empty/build-stats":
			_ = json.NewEncoder(w).Encode(apiclient.BuildStats{App: "empty"})
		case r.URL.Path == "/api/v1/build-limits" && r.Method == http.MethodPut:
			var b map[string]int
			_ = json.NewDecoder(r.Body).Decode(&b)
			putTimeout = b["timeout_seconds"]
			_ = json.NewEncoder(w).Encode(apiclient.BuildLimits{Effective: apiclient.BuildLimitValues{MaxConcurrent: 2, TimeoutSeconds: 90}})
		case r.URL.Path == "/api/v1/build-limits":
			_ = json.NewEncoder(w).Encode(apiclient.BuildLimits{Effective: apiclient.BuildLimitValues{MaxConcurrent: 0, TimeoutSeconds: 1800}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"stats", []string{"apps", "build-stats", "web"}, "p50 12s, p95 12s"},
		{"stats cache", []string{"apps", "build-stats", "web"}, "75% of steps cached"},
		{"stats empty", []string{"apps", "build-stats", "empty"}, "no builds recorded for empty yet"},
		{"limits show", []string{"apps", "build-limits", "show"}, "max concurrent: unlimited (default)"},
		{"limits set", []string{"apps", "build-limits", "set", "--timeout", "90s"}, "timeout:"},
		{"limits reset", []string{"apps", "build-limits", "reset"}, "30m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout %q missing %q", stdout, tt.want)
			}
		})
	}
	if putTimeout != 90 {
		t.Fatalf("PUT timeout_seconds = %d, want 90", putTimeout)
	}
}
