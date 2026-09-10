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
	testMetricsMissingMetric(t, []string{"databases", "metrics"}, "main")
}

func TestRun_DatabasesMetrics_NotFound(t *testing.T) {
	testMetricsNotFound(t, []string{"databases", "metrics"}, "missing", `{"error":"database not found"}`, "database not found")
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
	testMetricsHelp(t, []string{"databases", "metrics"}, "databases metrics")
}
