package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResourceChangeVersionsAndCAS(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	v1, err := db.RecordResourceChangeIf(ctx, ResourceChange{Kind: ResourceKindApp, Name: "web", Op: ResourceOpAdded}, 0)
	if err != nil {
		t.Fatalf("first CAS write: %v", err)
	}
	if _, err := db.RecordResourceChangeIf(ctx, ResourceChange{Kind: ResourceKindApp, Name: "web", Op: ResourceOpModified}, 0); !errors.Is(err, ErrResourceVersionConflict) {
		t.Fatalf("stale CAS write: got %v, want conflict", err)
	}
	v2, err := db.RecordResourceChangeIf(ctx, ResourceChange{Kind: ResourceKindApp, Name: "web", Op: ResourceOpModified}, v1)
	if err != nil {
		t.Fatalf("fresh CAS write: %v", err)
	}
	if v2 <= v1 {
		t.Fatalf("versions not monotonic: %d then %d", v1, v2)
	}
	cur, err := db.CurrentResourceVersion(ctx)
	if err != nil || cur != v2 {
		t.Fatalf("current version = %d, %v; want %d", cur, err, v2)
	}
	got, err := db.ResourceChangesSince(ctx, ResourceKindApp, v1, 0)
	if err != nil || len(got) != 1 || got[0].Version != v2 {
		t.Fatalf("changes since %d = %+v, %v", v1, got, err)
	}
}

func TestResourceChangeCompaction(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := db.RecordResourceChange(ctx, ResourceChange{Kind: ResourceKindApp, Name: "web", Op: ResourceOpModified}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.RecordResourceChange(ctx, ResourceChange{Kind: ResourceKindApp, Name: "api", Op: ResourceOpAdded}); err != nil {
		t.Fatal(err)
	}
	removed, err := db.CompactResourceChanges(ctx, ResourceRetention{Retain: 2, MaxAge: time.Hour}, time.Now())
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if removed != 4 {
		t.Fatalf("removed %d rows, want 4 (web 1..4; web 5 and api 6 kept)", removed)
	}
	mark, _ := db.ResourceCompactedThrough(ctx)
	if mark != 4 {
		t.Fatalf("watermark = %d, want 4", mark)
	}
	if _, err := db.ResourceChangesSince(ctx, ResourceKindApp, 0, 0); !errors.Is(err, ErrResourceVersionCompacted) {
		t.Fatalf("watch from 0 after compaction: got %v, want compacted", err)
	}
	latest, err := db.LatestResourceVersions(ctx, ResourceKindApp)
	if err != nil || latest["web"] != 5 || latest["api"] != 6 {
		t.Fatalf("latest versions = %v, %v", latest, err)
	}

	removed, err = db.CompactResourceChanges(ctx, ResourceRetention{Retain: 100, MaxAge: time.Minute}, time.Now().Add(time.Hour))
	if err != nil || removed != 0 {
		t.Fatalf("age compaction removed %d, %v; latest rows must survive", removed, err)
	}
	if mark, _ := db.ResourceCompactedThrough(ctx); mark != 6 {
		t.Fatalf("watermark after age compaction = %d, want 6", mark)
	}
}
