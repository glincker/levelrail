package store

import (
	"context"
	"testing"
	"time"
)

func TestAdvisorFindingStateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	steps := []AdvisorFindingState{
		{FindingID: "memory_limit_missing:app:web", State: AdvisorFindingSnoozed, SnoozedUntil: now.Add(24 * time.Hour), Actor: "admin", UpdatedAt: now},
		{FindingID: "memory_limit_missing:app:web", State: AdvisorFindingDismissed, Actor: "admin", UpdatedAt: now},
	}
	for _, s := range steps {
		if err := db.SetAdvisorFindingState(ctx, s); err != nil {
			t.Fatal(err)
		}
		got, err := db.ListAdvisorFindingStates(ctx)
		if err != nil {
			t.Fatal(err)
		}
		g := got[s.FindingID]
		if g.State != s.State || !g.SnoozedUntil.Equal(s.SnoozedUntil) || !g.Hides(now) {
			t.Fatalf("got %+v, want %+v", g, s)
		}
	}
	if err := db.ClearAdvisorFindingState(ctx, "memory_limit_missing:app:web"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListAdvisorFindingStates(ctx); len(got) != 0 {
		t.Fatalf("clear left %v", got)
	}
}

func TestAdvisorFindingStateHides(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		s    AdvisorFindingState
		want bool
	}{
		{"dismissed", AdvisorFindingState{State: AdvisorFindingDismissed}, true},
		{"snoozed ahead", AdvisorFindingState{State: AdvisorFindingSnoozed, SnoozedUntil: now.Add(time.Hour)}, true},
		{"snooze over", AdvisorFindingState{State: AdvisorFindingSnoozed, SnoozedUntil: now.Add(-time.Hour)}, false},
	}
	for _, tc := range tests {
		if got := tc.s.Hides(now); got != tc.want {
			t.Errorf("%s: Hides = %v", tc.name, got)
		}
	}
}

func TestBetaFeatureSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if got, err := db.ListBetaFeatures(ctx); err != nil || len(got) != 0 {
		t.Fatalf("fresh list = %v, %v", got, err)
	}
	now := time.Now()
	if err := db.SetBetaFeature(ctx, "resource-advisor", true, "admin", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetBetaFeature(ctx, "resource-advisor", false, "admin", now); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListBetaFeatures(ctx)
	if err != nil || got["resource-advisor"] {
		t.Fatalf("list = %v, %v", got, err)
	}
	if _, ok := got["resource-advisor"]; !ok {
		t.Fatal("row missing after toggle off")
	}
}
