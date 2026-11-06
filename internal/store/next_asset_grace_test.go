package store

import (
	"context"
	"testing"
	"time"
)

func TestNextAssetGraceRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for _, g := range []NextAssetGrace{
		{Service: "web", DeploymentID: "old", Image: "img1", Until: now.Add(time.Hour)},
		{Service: "web", DeploymentID: "older", Image: "img0", Until: now.Add(-time.Minute)},
	} {
		if err := db.InsertNextAssetGrace(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	active, err := db.ListActiveNextAssetGrace(ctx, now)
	if err != nil || len(active) != 1 || active[0].DeploymentID != "old" || !active[0].Until.Equal(now.Add(time.Hour)) {
		t.Fatalf("active = %+v, err %v", active, err)
	}
	if err := db.DeleteExpiredNextAssetGrace(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteNextAssetGraceForService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if active, _ = db.ListActiveNextAssetGrace(ctx, now); len(active) != 0 {
		t.Fatalf("want empty, got %+v", active)
	}
}
