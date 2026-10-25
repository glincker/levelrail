package store

import "testing"

func TestListStackOwnerNames(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestStack(t, db, "shop-prod")
	for _, e := range []StackLedgerEntry{
		{StackID: a.ID, Kind: "App", Key: "App/web", LiveID: "web", SinceRevision: 1},
		{StackID: a.ID, Kind: "Database", Key: "Database/pg", LiveID: "pg", SinceRevision: 1},
	} {
		if err := db.UpsertStackResource(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListStackOwnerNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got[StackOwnerKey("App", "App/web")] != "shop-prod" || got[StackOwnerKey("Database", "Database/pg")] != "shop-prod" || len(got) != 2 {
		t.Fatalf("owners = %v", got)
	}
}
