package store

import (
	"testing"
	"time"
)

func TestVisitorRollupRoundTripsClassesAndPages(t *testing.T) {
	db := openTestDB(t)
	hour := time.Now().Add(-2 * time.Hour).Truncate(time.Hour).UTC()
	in := VisitorRollup{
		App: "web", Domain: "a.test", HourStart: hour, Epoch: 7, Requests: 9, Visitors: 2,
		Classes:  [6]int{1, 4, 3, 1, 0, 0},
		TopPages: []RollupCount{{Label: "/pricing", Count: 4}},
		TopPaths: []RollupCount{{Label: "/api/x", Count: 3}},
	}
	if err := db.UpsertVisitorRollups(t.Context(), []VisitorRollup{in}); err != nil {
		t.Fatal(err)
	}
	// An upsert on the same key replaces the row.
	in.Classes[1] = 5
	if err := db.UpsertVisitorRollups(t.Context(), []VisitorRollup{in}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListVisitorRollups(t.Context(), "web", "", hour.Add(-time.Hour), time.Now())
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d, err %v", len(rows), err)
	}
	if rows[0].Classes != [6]int{1, 5, 3, 1, 0, 0} || len(rows[0].TopPages) != 1 || rows[0].TopPages[0].Label != "/pricing" || rows[0].TopPaths[0].Label != "/api/x" {
		t.Fatalf("row = %+v", rows[0])
	}
}
