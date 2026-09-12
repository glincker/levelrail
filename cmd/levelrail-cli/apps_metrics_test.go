package main

import (
	"strings"
	"testing"
)

func TestRun_AppsMetrics(t *testing.T) {
	testMetricsQuerySuccess(t, []string{"apps", "metrics"}, "web", "/api/v1/apps/web/metrics", "cpu_percent", 12.5, "12.5")
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
	testMetricsNoName(t, []string{"apps", "metrics"})
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
