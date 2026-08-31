package main

import (
	"strings"
	"testing"
)

func TestRunDatabasesResourceRecommendation_JSON(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, resourceRecommendationResource{
		ServiceName:    "main",
		LookbackWindow: "168h0m0s",
		Memory: dimensionRecommendationResource{
			Dimension: "memory", SampleCount: 200, DataSufficient: true, Confidence: "high",
			CurrentLimit: 512 * 1024 * 1024, P95Usage: 480 * 1024 * 1024, SuggestedLimit: 624 * 1024 * 1024,
			Action: "raise", Reason: "p95 usage is close to the current limit",
		},
		CPU: dimensionRecommendationResource{
			Dimension: "cpu", SampleCount: 200, DataSufficient: true, Confidence: "high",
			Reason: "usage is comfortably within the current limit",
		},
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"databases", "resource-recommendation", "main", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"action": "raise"`) {
		t.Errorf("stdout = %q, want it to contain the memory action field", stdout)
	}
	if gotPath != "/api/v1/databases/main/resource-recommendation" {
		t.Errorf("request path = %q, want /api/v1/databases/main/resource-recommendation", gotPath)
	}
}

func TestRunDatabasesResourceRecommendation_Human(t *testing.T) {
	srv := newListEchoServer(t, nil, resourceRecommendationResource{
		ServiceName:    "main",
		LookbackWindow: "168h0m0s",
		OOMDetectedAt:  "2026-08-25T10:00:00Z",
		OOMExcerpt:     "container killed: oomkilled",
		Memory: dimensionRecommendationResource{
			Dimension: "memory", SampleCount: 5, Confidence: "high",
			CurrentLimit: 256 * 1024 * 1024, SuggestedLimit: 384 * 1024 * 1024,
			Action: "raise", Reason: "This database was OOM-killed on 2026-08-25T10:00:00Z.",
		},
		CPU: dimensionRecommendationResource{
			Dimension: "cpu", SampleCount: 5, Confidence: "low",
			Reason: "Not enough historical data yet.",
		},
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"databases", "resource-recommendation", "main", "--api-url", srv.URL})
	if !strings.Contains(stdout, "database:") {
		t.Errorf("stdout = %q, want the database label", stdout)
	}
	if !strings.Contains(stdout, "OOM signal") {
		t.Errorf("stdout = %q, want the OOM signal called out", stdout)
	}
	if !strings.Contains(stdout, "raise") {
		t.Errorf("stdout = %q, want the suggested action", stdout)
	}
	if !strings.Contains(stdout, "Not enough historical data") {
		t.Errorf("stdout = %q, want the CPU reason text", stdout)
	}
}

func TestRunDatabasesResourceRecommendation_MissingName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "resource-recommendation"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}
