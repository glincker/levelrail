package store

import (
	"context"
	"testing"
	"time"
)

func TestDeployPinSurvivesJanitorUntilUnpinned(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"pinned-failed", "plain-failed"} {
		a := DeployAttempt{ID: id, ServiceName: "web", Image: "web:" + id, Source: DeployAttemptSourceImage, Status: DeployAttemptStatusFailed, StartedAt: now.Add(-60 * 24 * time.Hour)}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.PinDeploy(ctx, "web", "pinned-failed", "tester"); err != nil {
		t.Fatal(err)
	}
	if err := db.PinDeploy(ctx, "web", "pinned-failed", "other"); err != nil {
		t.Fatal(err)
	}
	cutoff := now.Add(-30 * 24 * time.Hour)
	got, err := db.ListPrunableDeployAttempts(ctx, cutoff, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "plain-failed" {
		t.Fatalf("prunable = %+v, want only plain-failed", got)
	}
	if err := db.UnpinDeploy(ctx, "pinned-failed"); err != nil {
		t.Fatal(err)
	}
	got, err = db.ListPrunableDeployAttempts(ctx, cutoff, 0, 100)
	if err != nil || len(got) != 2 {
		t.Fatalf("after unpin prunable = %+v, %v, want both", got, err)
	}
}
