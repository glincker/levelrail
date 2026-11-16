package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestForgeStatusReportsOrderCursorAndRetention(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for i, state := range []string{"pending", "success", "failure"} {
		err := db.RecordForgeStatusReport(ctx, ForgeStatusReport{
			AppName: "web", SHA: "abc1234", State: state, Context: "levelrail/deploy",
			Source: ForgeStatusSourcePipeline, CreatedAt: now.Add(-time.Duration(3-i) * 24 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordForgeStatusReport(ctx, ForgeStatusReport{AppName: "other", SHA: "f", State: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordForgeStatusReport(ctx, ForgeStatusReport{AppName: "web"}); err == nil {
		t.Fatal("expected validation error")
	}

	got, err := db.ListForgeStatusReports(ctx, "web", nil, 10)
	if err != nil || len(got) != 3 || got[0].State != "failure" || got[2].State != "pending" {
		t.Fatalf("list = %+v, %v", got, err)
	}
	next, err := db.ListForgeStatusReports(ctx, "web", &AppEventCursor{At: got[0].CreatedAt, ID: got[0].ID}, 10)
	if err != nil || len(next) != 2 || next[0].State != "success" {
		t.Fatalf("cursor = %+v, %v", next, err)
	}
	empty, err := db.ListForgeStatusReports(ctx, "nobody", nil, 10)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty must be a non-nil empty slice: %#v, %v", empty, err)
	}

	cutoff := now.Add(-36 * time.Hour)
	n, err := db.RetentionDeleteOlder(ctx, RetentionForgeStatus, cutoff, 100)
	if err != nil || n != 2 {
		t.Fatalf("retention deleted %d, %v", n, err)
	}
	left, _ := db.ListForgeStatusReports(ctx, "web", nil, 10)
	if len(left) != 1 || left[0].State != "failure" {
		t.Fatalf("left = %+v", left)
	}
}

func TestForgeStatusReportClipsLongText(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	long := strings.Repeat("x", 2000)
	if err := db.RecordForgeStatusReport(ctx, ForgeStatusReport{AppName: "web", SHA: "a", State: "failure", Error: long, Description: long}); err != nil {
		t.Fatal(err)
	}
	got, _ := db.ListForgeStatusReports(ctx, "web", nil, 1)
	if len(got) != 1 || len(got[0].Error) != forgeStatusTextMax || len(got[0].Description) != forgeStatusTextMax {
		t.Fatalf("not clipped: %+v", got)
	}
}
