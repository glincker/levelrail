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
