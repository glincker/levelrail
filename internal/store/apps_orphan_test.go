package store

import (
	"context"
	"testing"
)

// The sweep must keep an app that gained a member after an earlier empty listing.
func TestDeleteAppIfOrphanedKeepsLinkedApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveApp(ctx, App{ID: "x", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "x", Image: "img:1", Port: 80, Replicas: 1}); err != nil {
		t.Fatal(err)
	}
	stale, err := db.ListServicesByApp(ctx, "x")
	if err != nil || len(stale) != 0 {
		t.Fatalf("precondition: %v %v", stale, err)
	}
	if err := db.UpdateServiceApp(ctx, "x", "x"); err != nil {
		t.Fatal(err)
	}
	deleted, err := db.DeleteAppIfOrphaned(ctx, "x")
	if err != nil || deleted {
		t.Fatalf("linked app deleted=%v err=%v", deleted, err)
	}
	if _, err := db.GetDesiredService(ctx, "x"); err != nil {
		t.Fatalf("service lost: %v", err)
	}
	if err := db.DeleteDesiredService(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if deleted, err = db.DeleteAppIfOrphaned(ctx, "x"); err != nil || !deleted {
		t.Fatalf("orphan deleted=%v err=%v", deleted, err)
	}
}
