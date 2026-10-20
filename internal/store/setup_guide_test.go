package store

import (
	"context"
	"testing"
)

func TestSetupGuideStateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetSetupGuideState(ctx)
	if err != nil {
		t.Fatalf("GetSetupGuideState() error = %v", err)
	}
	if got.Hidden || len(got.Overrides) != 0 {
		t.Fatalf("default state = %+v, want visible and no overrides", got)
	}

	if err := db.SetSetupGuideHidden(ctx, true, "u1"); err != nil {
		t.Fatalf("SetSetupGuideHidden() error = %v", err)
	}
	if err := db.SetSetupGuideOverride(ctx, "teammate", SetupGuideOverrideSkipped, "u1"); err != nil {
		t.Fatalf("SetSetupGuideOverride(skip) error = %v", err)
	}
	if err := db.SetSetupGuideOverride(ctx, "teammate", SetupGuideOverrideDone, "u1"); err != nil {
		t.Fatalf("SetSetupGuideOverride(done) error = %v", err)
	}
	got, _ = db.GetSetupGuideState(ctx)
	if !got.Hidden || got.Overrides["teammate"] != SetupGuideOverrideDone {
		t.Fatalf("state = %+v, want hidden with teammate=done", got)
	}

	if err := db.SetSetupGuideOverride(ctx, "teammate", "", "u1"); err != nil {
		t.Fatalf("clear override error = %v", err)
	}
	got, _ = db.GetSetupGuideState(ctx)
	if len(got.Overrides) != 0 {
		t.Fatalf("overrides after clear = %v, want none", got.Overrides)
	}

	_ = db.SetSetupGuideOverride(ctx, "tls", SetupGuideOverrideSkipped, "u1")
	if err := db.ResetSetupGuide(ctx); err != nil {
		t.Fatalf("ResetSetupGuide() error = %v", err)
	}
	got, _ = db.GetSetupGuideState(ctx)
	if got.Hidden || len(got.Overrides) != 0 {
		t.Fatalf("state after reset = %+v, want default", got)
	}
}

func TestSetupGuideOverrideRejectsUnknownState(t *testing.T) {
	db := openTestDB(t)
	if err := db.SetSetupGuideOverride(context.Background(), "tls", "bogus", "u1"); err == nil {
		t.Fatal("want CHECK constraint error for unknown state")
	}
}
