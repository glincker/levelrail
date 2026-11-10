package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A published postmortem keeps its incident through retention and manual delete; a draft does not.
func TestIncidentPostmortem_PublishedSurvivesRetentionAndDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"keep", "drop"} {
		in := InternalStatusIncident{
			StatusIncident:         StatusIncident{ID: id, Kind: "incident", Title: id, Status: "investigating", Impact: "none", StartsAt: now},
			StatusIncidentInternal: StatusIncidentInternal{Source: "monitor", InternalState: IncidentStateResolved},
		}
		if err := db.CreateInternalStatusIncident(ctx, in); err != nil {
			t.Fatal(err)
		}
		if err := db.DraftIncidentPostmortem(ctx, IncidentPostmortem{IncidentID: id, ImpactStart: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.PublishIncidentPostmortem(ctx, "keep", "op", now); !errors.Is(err, ErrIncidentPostmortemIncomplete) {
		t.Fatalf("publish empty = %v, want incomplete", err)
	}
	s, rc := "s", "rc"
	if err := db.EditIncidentPostmortem(ctx, "keep", PostmortemEdit{Summary: &s, RootCause: &rc}, "op", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishIncidentPostmortem(ctx, "keep", "op", now); err != nil {
		t.Fatal(err)
	}
	if err := db.EditIncidentPostmortem(ctx, "keep", PostmortemEdit{Summary: &rc}, "op", now); !errors.Is(err, ErrIncidentPostmortemPublished) {
		t.Fatalf("edit after publish = %v", err)
	}
	if n, err := db.RetentionDeleteOlder(ctx, RetentionStatusIncidents, now.Add(time.Hour), 100); err != nil || n != 1 {
		t.Fatalf("retention deleted %d (%v), want only the draft incident", n, err)
	}
	if _, err := db.GetInternalStatusIncident(ctx, "keep"); err != nil {
		t.Fatalf("published incident gone after retention: %v", err)
	}
	if err := db.DeleteStatusIncident(ctx, "keep"); !errors.Is(err, ErrIncidentPostmortemPublished) {
		t.Fatalf("delete = %v, want published refusal", err)
	}
}
