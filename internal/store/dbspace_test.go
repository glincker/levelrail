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
	got := map[string]int64{}
	for _, p := range rep.Parts {
		got[p.ID] = p.Bytes
	}
	for _, id := range []string{SpacePartDeploys, SpacePartEvents, SpacePartAudit} {
		if len(rep.Tables) > 0 && got[id] == 0 {
			t.Fatalf("part %s missing or empty in %+v", id, rep.Parts)
		}
	}
	if err := db.CompactDatabase(ctx); err != nil {
		t.Fatal(err)
	}
}
