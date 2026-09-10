package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsMetrics(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appMetricsResource{
			Metric: "cpu_percent",
			Points: []metricPointResource{{Value: 12.5, Count: 3}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "metrics", "web", "--metric", "cpu_percent", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/metrics" {
		t.Errorf("path = %q, want /api/v1/apps/web/metrics", gotPath)
	}
	if !strings.Contains(gotQuery, "metric=cpu_percent") {
		t.Errorf("query = %q, want metric=cpu_percent", gotQuery)
	}
	if !strings.Contains(stdout, "cpu_percent") || !strings.Contains(stdout, "12.5") {
		t.Errorf("stdout = %q, want the metric name and point value", stdout)
	}
}

func TestRun_AppsMetrics_JSON(t *testing.T) {
	srv := newListEchoServer(t, nil, appMetricsResource{
		Metric: "cpu_percent",
		Points: []metricPointResource{{Value: 1, Count: 1}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "metrics", "web", "--metric", "cpu_percent", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"metric": "cpu_percent"`) {
		t.Errorf("stdout = %q, want the metrics resource as JSON", stdout)
	}
}

func TestRun_AppsMetrics_EmptyPoints(t *testing.T) {
	srv := newListEchoServer(t, nil, appMetricsResource{Metric: "cpu_percent"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "metrics", "web", "--metric", "cpu_percent", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no data points in range") {
		t.Errorf("stdout = %q, want the no-data-points message", stdout)
	}
}

func TestRun_AppsMetrics_MissingMetric(t *testing.T) {
	testMetricsMissingMetric(t, []string{"apps", "metrics"}, "web")
}

func TestRun_AppsMetrics_InvalidStep(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "metrics", "web", "--metric", "cpu_percent", "--step", "not-a-duration"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--step must be a valid duration") {
		t.Errorf("stderr = %q, want a --step validation error", stderr.String())
	}
}

func TestRun_AppsMetrics_NotFound(t *testing.T) {
	testMetricsNotFound(t, []string{"apps", "metrics"}, "missing", `{"error":"app not found"}`, "app not found")
}

func TestRun_AppsMetrics_NoName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "metrics"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsMetrics_Help(t *testing.T) {
	testMetricsHelp(t, []string{"apps", "metrics"}, "apps metrics")
}

func TestRun_AppsMetrics_Query(t *testing.T) {
	srv := newListEchoServer(t, nil, appMetricsResource{
		Metric: "cpu_percent",
		Points: []metricPointResource{{Value: 7, Count: 1}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "metrics", "web", "--metric", "cpu_percent", "--api-url", srv.URL, "--query", "metric"})
	if !strings.Contains(stdout, "cpu_percent") {
		t.Errorf("stdout = %q, want the projected metric field", stdout)
	}
}
