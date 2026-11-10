package store

import (
	"context"
	"testing"
)

func TestSpaceReportAndCompact(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	rep, err := db.SpaceReport(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PageSize == 0 || rep.TotalBytes != rep.PageSize*rep.PageCount {
		t.Fatalf("unexpected report %+v", rep)
	}
	if err := db.CompactDatabase(ctx); err != nil {
		t.Fatal(err)
	}
}
