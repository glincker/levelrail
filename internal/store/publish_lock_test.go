package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPublishLockAndHeldBuilds(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()

	first, err := db.SetPublishLock(ctx, PublishLock{ServiceName: "web", Reason: "a", LockedBy: "x", LockedAt: now, PinnedImage: "web:1"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.SetPublishLock(ctx, PublishLock{ServiceName: "web", Reason: "b", LockedBy: "y", LockedAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if again.Reason != first.Reason || again.LockedBy != "x" {
		t.Fatalf("second lock overwrote the first: %+v", again)
	}

	if err := db.SaveHeldBuild(ctx, "d1", DesiredService{Name: "web", Image: "web:2", Port: 80}, now); err != nil {
		t.Fatal(err)
	}
	if ids, _ := db.ListHeldBuildIDs(ctx, "web"); !ids["d1"] {
		t.Fatalf("held ids = %v", ids)
	}
	svc, err := db.GetHeldBuild(ctx, "d1")
	if err != nil || svc.Image != "web:2" {
		t.Fatalf("GetHeldBuild = %+v, %v", svc, err)
	}
	for i := 0; i < 2; i++ {
		if err := db.MarkHeldBuildPublished(ctx, "d1", "x", now); err != nil {
			t.Fatalf("mark %d: %v", i, err)
		}
	}
	if _, err := db.GetHeldBuild(ctx, "d1"); !errors.Is(err, ErrHeldBuildNotFound) {
		t.Fatalf("published build still readable: %v", err)
	}
	if cleared, _ := db.ClearPublishLock(ctx, "web"); !cleared {
		t.Fatal("clear reported not locked")
	}
	if cleared, _ := db.ClearPublishLock(ctx, "web"); cleared {
		t.Fatal("second clear reported locked")
	}
}
