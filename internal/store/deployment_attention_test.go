package store

import (
	"context"
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

func TestListDeploymentsMarksFirstAttemptAndBriefsHonorEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:0", Port: 80}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	for i, id := range []string{"dep_a", "dep_b"} {
		if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: id, ServiceName: "web", Image: "web:1", Source: DeployAttemptSourceImage,
			Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}
	rows, _, err := db.ListDeployments(ctx, DeploymentFilter{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %d, err = %v", len(rows), err)
	}
	if rows[0].First || !rows[1].First {
		t.Fatalf("first flags newest=%v oldest=%v, want false/true", rows[0].First, rows[1].First)
	}
	all, err := db.ListDeploymentBriefsScoped(ctx, base.Add(-time.Hour), nil, "", "")
	if err != nil || len(all) != 2 {
		t.Fatalf("unscoped briefs = %d, err = %v", len(all), err)
	}
	none, err := db.ListDeploymentBriefsScoped(ctx, base.Add(-time.Hour), nil, "no-such-env", "")
	if err != nil || len(none) != 0 {
		t.Fatalf("scoped briefs = %d, err = %v, want 0", len(none), err)
	}
}
