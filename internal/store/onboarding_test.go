package store

import (
	"context"
	"testing"
)

func TestGetOnboardingCompleted_DefaultsFalse(t *testing.T) {
	db := openTestDB(t)

	completed, err := db.GetOnboardingCompleted(context.Background())
	if err != nil {
		t.Fatalf("GetOnboardingCompleted: %v", err)
	}
	if completed {
		t.Error("completed = true, want false on a freshly migrated database")
	}
}

func TestMarkOnboardingCompleted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.MarkOnboardingCompleted(ctx); err != nil {
		t.Fatalf("MarkOnboardingCompleted: %v", err)
	}

	completed, err := db.GetOnboardingCompleted(ctx)
	if err != nil {
		t.Fatalf("GetOnboardingCompleted: %v", err)
	}
	if !completed {
		t.Error("completed = false, want true after MarkOnboardingCompleted")
	}
}

func TestSetOnboardingProgress_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	initial, err := db.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("GetOnboardingState: %v", err)
	}
	if initial.CurrentStep != "" || len(initial.Steps) != 0 {
		t.Fatalf("initial state = %+v, want empty progress", initial)
	}

	steps := map[string]string{"server": "completed", "domain": "skipped"}
	if err := db.SetOnboardingProgress(ctx, "git", steps); err != nil {
		t.Fatalf("SetOnboardingProgress: %v", err)
	}
	got, err := db.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("GetOnboardingState: %v", err)
	}
	if got.CurrentStep != "git" {
		t.Errorf("CurrentStep = %q, want git", got.CurrentStep)
	}
	if got.Steps["server"] != "completed" || got.Steps["domain"] != "skipped" || len(got.Steps) != 2 {
		t.Errorf("Steps = %v, want %v", got.Steps, steps)
	}
	if got.Completed {
		t.Error("Completed = true, want false: progress alone never completes onboarding")
	}

	if err := db.SetOnboardingProgress(ctx, "", nil); err != nil {
		t.Fatalf("SetOnboardingProgress(nil): %v", err)
	}
	got, err = db.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("GetOnboardingState: %v", err)
	}
	if len(got.Steps) != 0 {
		t.Errorf("Steps = %v, want empty after reset", got.Steps)
	}
}
