package main

import "testing"

func TestRun_DatabasesMetrics(t *testing.T) {
	testMetricsQuerySuccess(t, []string{"databases", "metrics"}, "main", "/api/v1/databases/main/metrics", "memory_usage_bytes", 2048, "2048")
}

func TestRun_DatabasesMetrics_MissingMetric(t *testing.T) {
	testMetricsMissingMetric(t, []string{"databases", "metrics"}, "main")
}

func TestRun_DatabasesMetrics_NotFound(t *testing.T) {
	testMetricsNotFound(t, []string{"databases", "metrics"}, "missing", `{"error":"database not found"}`, "database not found")
}

func TestRun_DatabasesMetrics_NoName(t *testing.T) {
	testMetricsNoName(t, []string{"databases", "metrics"})
}

func TestRun_DatabasesMetrics_Help(t *testing.T) {
	testMetricsHelp(t, []string{"databases", "metrics"}, "databases metrics")
}
