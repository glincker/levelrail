package store

import (
	"context"
	"testing"
	"time"
)

func TestAddStatusIncidentComponentsMerges(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	in := InternalStatusIncident{
		StatusIncident:         StatusIncident{ID: "inc_1", Kind: "incident", Title: "t", Status: "open", Impact: "minor", ComponentIDs: []string{"c1"}, StartsAt: time.Now()},
		StatusIncidentInternal: StatusIncidentInternal{Source: "monitor", InternalState: IncidentStateTriggered},
	}
	if err := db.CreateInternalStatusIncident(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := db.AddStatusIncidentComponents(ctx, "inc_1", []string{"c1", "c2"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetInternalStatusIncident(ctx, "inc_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ComponentIDs) != 2 || got.ComponentIDs[1] != "c2" {
		t.Fatalf("components = %v", got.ComponentIDs)
	}
}
