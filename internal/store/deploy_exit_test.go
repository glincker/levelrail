package store

import (
	"context"
	"testing"
	"time"
)

func TestDeployExitCaptures(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:2", Port: 80}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	for i, img := range []string{"web:1", "web:2"} {
		err := db.SaveDeployAttempt(ctx, DeployAttempt{
			ID: "dep_" + img[4:], ServiceName: "web", Image: img, Status: DeployAttemptStatusSucceeded,
			StartedAt: now.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}

	if got, err := db.LatestDeployExit(ctx, "web"); err != nil || got != nil {
		t.Fatalf("LatestDeployExit before any capture = %+v, %v; want nil, nil", got, err)
	}

	first := DeployExit{
		ServiceName: "web", Image: "web:2", Container: "web-abc", ExitCode: 0, Reason: "ExitedDuringReadiness",
		Message: "Container exited with code 0 after 2s, before it became ready", StartedAt: now, FinishedAt: now.Add(2 * time.Second),
		LogTail: []DeployExitLine{{Timestamp: now, Stream: "stdout", Message: "usage: web [serve]"}}, CapturedAt: now.Add(3 * time.Second),
	}
	if err := db.RecordDeployExit(ctx, first); err != nil {
		t.Fatalf("RecordDeployExit: %v", err)
	}
	got, err := db.GetDeployExit(ctx, "web", "dep_2")
	if err != nil || got == nil {
		t.Fatalf("GetDeployExit = %+v, %v", got, err)
	}
	if got.AttemptID != "dep_2" || got.Message != first.Message || len(got.LogTail) != 1 || got.LogTail[0].Message != "usage: web [serve]" {
		t.Fatalf("capture round trip mismatch: %+v", got)
	}
	if !got.FinishedAt.Equal(first.FinishedAt) {
		t.Fatalf("FinishedAt = %v, want %v", got.FinishedAt, first.FinishedAt)
	}

	// A crash loop replaces the attempt's capture rather than piling up rows.
	second := first
	second.ExitCode, second.Container, second.CapturedAt, second.LogTail = 1, "web-def", now.Add(time.Minute), nil
	if err := db.RecordDeployExit(ctx, second); err != nil {
		t.Fatalf("RecordDeployExit again: %v", err)
	}
	latest, err := db.LatestDeployExit(ctx, "web")
	if err != nil || latest == nil || latest.ExitCode != 1 || latest.Container != "web-def" || len(latest.LogTail) != 0 {
		t.Fatalf("LatestDeployExit = %+v, %v; want the replacement", latest, err)
	}
	if none, err := db.GetDeployExit(ctx, "web", "dep_1"); err != nil || none != nil {
		t.Fatalf("other attempt capture = %+v, %v; want nil", none, err)
	}

	orphan := first
	orphan.Image = "web:unknown"
	if err := db.RecordDeployExit(ctx, orphan); err != nil {
		t.Fatalf("RecordDeployExit without attempt: %v", err)
	}
	if got, err := db.GetDeployExit(ctx, "web", ""); err != nil || got == nil {
		t.Fatalf("unmatched capture = %+v, %v; want stored under empty attempt", got, err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService: %v", err)
	}
	if got, err := db.LatestDeployExit(ctx, "web"); err != nil || got != nil {
		t.Fatalf("captures after delete = %+v, %v; want none", got, err)
	}
}
