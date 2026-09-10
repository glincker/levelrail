package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesMetrics(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appMetricsResource{
			Metric: "memory_usage_bytes",
			Points: []metricPointResource{{Value: 2048, Count: 1}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "metrics", "main", "--metric", "memory_usage_bytes", "--api-url", srv.URL})
	if gotPath != "/api/v1/databases/main/metrics" {
		t.Errorf("path = %q, want /api/v1/databases/main/metrics", gotPath)
	}
	if !strings.Contains(gotQuery, "metric=memory_usage_bytes") {
		t.Errorf("query = %q, want metric=memory_usage_bytes", gotQuery)
	}
	if !strings.Contains(stdout, "memory_usage_bytes") || !strings.Contains(stdout, "2048") {
		t.Errorf("stdout = %q, want the metric name and point value", stdout)
	}
}

func TestRun_DatabasesMetrics_MissingMetric(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "metrics", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--metric is required") {
		t.Errorf("stderr = %q, want a missing --metric validation error", stderr.String())
	}
}

func TestRun_DatabasesMetrics_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"database not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"databases", "metrics", "missing", "--metric", "cpu_percent", "--api-url", srv.URL})
	if !strings.Contains(stderr, "database not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_DatabasesMetrics_NoName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "metrics"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_DatabasesMetrics_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "metrics", "-h"})
	if !strings.Contains(stderr, "databases metrics") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
