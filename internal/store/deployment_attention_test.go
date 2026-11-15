package store

import (
	"testing"
	"time"
)

func TestBuildDeploymentSummaryAttentionCountsNewestFailedOrRefused(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	briefs := []DeploymentBrief{
		{Service: "a", Status: DeploymentReady, StartedAt: now.Add(-3 * time.Hour)},
		{Service: "a", Status: DeploymentRefused, StartedAt: now.Add(-time.Hour)},
		{Service: "b", Status: DeploymentFailed, StartedAt: now.Add(-2 * time.Hour)},
		{Service: "c", Status: DeploymentFailed, StartedAt: now.Add(-4 * time.Hour)},
		{Service: "c", Status: DeploymentReady, StartedAt: now.Add(-time.Hour)},
	}
	got := BuildDeploymentSummary(briefs, now, 24*time.Hour)
	if got.NeedsAttention != 2 {
		t.Fatalf("NeedsAttention = %d, want 2 (a refused, b failed)", got.NeedsAttention)
	}
	if got.Counts[DeploymentRefused] != 1 {
		t.Fatalf("refused count = %d, want 1", got.Counts[DeploymentRefused])
	}
}
