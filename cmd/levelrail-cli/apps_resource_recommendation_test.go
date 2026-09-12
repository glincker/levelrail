package main

import (
	"fmt"
	"strings"
	"testing"
)

// testResourceRecommendationJSON runs "<subject> resource-recommendation
// <name> --json" and asserts the request path and that the memory action
// field is present. subject is "apps" or "databases".
func testResourceRecommendationJSON(t *testing.T, subject, name string) {
	t.Helper()
	var gotPath string
	srv := newListEchoServer(t, &gotPath, resourceRecommendationResource{
		ServiceName:    name,
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

	stdout, _ := runCLIExpectOK(t, []string{subject, "resource-recommendation", name, "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"action": "raise"`) {
		t.Errorf("stdout = %q, want it to contain the memory action field", stdout)
	}
	wantPath := fmt.Sprintf("/api/v1/%s/%s/resource-recommendation", subject, name)
	if gotPath != wantPath {
		t.Errorf("request path = %q, want %s", gotPath, wantPath)
	}
}

// testResourceRecommendationHuman runs "<subject> resource-recommendation
// <name>" and asserts the OOM signal, suggested action, and CPU reason
// appear in the human-readable output. oomSubject names the resource kind
// in the expected OOM reason text (e.g. "app" or "database"). It returns
// stdout so callers can add resource-specific assertions.
func testResourceRecommendationHuman(t *testing.T, subject, name, oomSubject string) string {
	t.Helper()
	srv := newListEchoServer(t, nil, resourceRecommendationResource{
		ServiceName:    name,
		LookbackWindow: "168h0m0s",
		OOMDetectedAt:  "2026-08-25T10:00:00Z",
		OOMExcerpt:     "container killed: oomkilled",
		Memory: dimensionRecommendationResource{
			Dimension: "memory", SampleCount: 5, Confidence: "high",
			CurrentLimit: 256 * 1024 * 1024, SuggestedLimit: 384 * 1024 * 1024,
			Action: "raise", Reason: fmt.Sprintf("This %s was OOM-killed on 2026-08-25T10:00:00Z.", oomSubject),
		},
		CPU: dimensionRecommendationResource{
			Dimension: "cpu", SampleCount: 5, Confidence: "low",
			Reason: "Not enough historical data yet.",
		},
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{subject, "resource-recommendation", name, "--api-url", srv.URL})
	if !strings.Contains(stdout, "OOM signal") {
		t.Errorf("stdout = %q, want the OOM signal called out", stdout)
	}
	if !strings.Contains(stdout, "raise") {
		t.Errorf("stdout = %q, want the suggested action", stdout)
	}
	if !strings.Contains(stdout, "Not enough historical data") {
		t.Errorf("stdout = %q, want the CPU reason text", stdout)
	}
	return stdout
}

// testResourceRecommendationMissingName runs "<subject>
// resource-recommendation" with no name and asserts the usage error.
func testResourceRecommendationMissingName(t *testing.T, subject string) {
	t.Helper()
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{subject, "resource-recommendation"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRunAppsResourceRecommendation_JSON(t *testing.T) {
	testResourceRecommendationJSON(t, "apps", "web")
}

func TestRunAppsResourceRecommendation_Human(t *testing.T) {
	testResourceRecommendationHuman(t, "apps", "web", "app")
}

func TestRunAppsResourceRecommendation_MissingName(t *testing.T) {
	testResourceRecommendationMissingName(t, "apps")
}
