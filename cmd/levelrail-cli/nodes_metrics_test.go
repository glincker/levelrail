package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesMetrics(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeMetricsResource{
			Metric:        "cpu_percent",
			Points:        []metricPointResource{{Value: 42, Count: 2}},
			ResourceCount: 2,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "metrics", "nd_1", "--metric", "cpu_percent", "--api-url", srv.URL})
	if gotPath != "/api/v1/nodes/nd_1/metrics" {
		t.Errorf("path = %q, want /api/v1/nodes/nd_1/metrics", gotPath)
	}
	if !strings.Contains(gotQuery, "metric=cpu_percent") {
		t.Errorf("query = %q, want metric=cpu_percent", gotQuery)
	}
	if !strings.Contains(stdout, "resource_count: 2") {
		t.Errorf("stdout = %q, want the resource_count line", stdout)
	}
	if !strings.Contains(stdout, "cpu_percent") || !strings.Contains(stdout, "42") {
		t.Errorf("stdout = %q, want the metric name and point value", stdout)
	}
}

func TestRun_NodesMetrics_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeMetricsResource{Metric: "cpu_percent", ResourceCount: 0})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "metrics", "nd_1", "--metric", "cpu_percent", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"resource_count": 0`) {
		t.Errorf("stdout = %q, want resource_count in the JSON output", stdout)
	}
}

func TestRun_NodesMetrics_MissingMetric(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"nodes", "metrics", "nd_1"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--metric is required") {
		t.Errorf("stderr = %q, want a missing --metric validation error", stderr.String())
	}
}

func TestRun_NodesMetrics_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "metrics", "nd_missing", "--metric", "cpu_percent", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesMetrics_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "metrics")
}

func TestRun_NodesMetrics_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "metrics", "-h"})
	if !strings.Contains(stderr, "nodes metrics") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
