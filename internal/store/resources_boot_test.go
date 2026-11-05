package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAdvanceResourceBootExpiresEveryEarlierVersion(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "boot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, n := range []string{"a", "b", "c"} {
		if _, err := db.RecordResourceChange(ctx, ResourceChange{Kind: ResourceKindApp, Name: n, Op: ResourceOpAdded}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AdvanceResourceBoot(ctx, 1000); err != nil {
		t.Fatal(err)
	}

	head, err := db.CurrentResourceVersion(ctx)
	if err != nil || head != 1003 {
		t.Fatalf("head = %d err=%v", head, err)
	}
	// A client that saw version 1 or a version from a lost tail must relist.
	for _, after := range []int64{0, 1, 3, 500, 1002} {
		if _, err := db.ResourceChangesSince(ctx, ResourceKindApp, after, 10); !errors.Is(err, ErrResourceVersionCompacted) {
			t.Errorf("after %d: err = %v, want compacted", after, err)
		}
	}
	// A list taken now returns the head, which must resume cleanly.
	if _, err := db.ResourceChangesSince(ctx, ResourceKindApp, head, 10); err != nil {
		t.Fatalf("resume at head: %v", err)
	}
	v, err := db.RecordResourceChange(ctx, ResourceChange{Kind: ResourceKindApp, Name: "d", Op: ResourceOpAdded})
	if err != nil || v != 1004 {
		t.Fatalf("next version = %d err=%v", v, err)
	}
	got, err := db.ResourceChangesSince(ctx, ResourceKindApp, head, 10)
	if err != nil || len(got) != 1 || got[0].Name != "d" {
		t.Fatalf("changes after head = %+v err=%v", got, err)
	}
}

func TestAdvanceResourceBootLeavesAnEmptyLogAlone(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "boot0.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.AdvanceResourceBoot(ctx, 50); err != nil {
		t.Fatal(err)
	}
	v, err := db.RecordResourceChange(ctx, ResourceChange{Kind: ResourceKindApp, Name: "a", Op: ResourceOpAdded})
	if err != nil || v != 1 {
		t.Fatalf("first version = %d err=%v", v, err)
	}
	if _, err := db.ResourceChangesSince(ctx, ResourceKindApp, 0, 10); err != nil {
		t.Fatalf("watch from 0 must keep working: %v", err)
	}
	if err := db.AdvanceResourceBoot(ctx, -1); err == nil {
		t.Fatal("a negative gap must fail")
	}
}
