package pagespreviews

import (
	"testing"
	"time"
)

func TestPruneKeepsNewestAndCurrent(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	es := []Entry{
		{PreviewApp: "a", Branch: "x", UpdatedAt: base},
		{PreviewApp: "b", Branch: "y", UpdatedAt: base.Add(time.Hour)},
		{PreviewApp: "c", Branch: "z", UpdatedAt: base.Add(2 * time.Hour)},
	}
	got := Prune(es, 2, "a")
	if len(got) != 1 || got[0].PreviewApp != "b" {
		t.Fatalf("want only b pruned, got %+v", got)
	}
	if Prune(es, 0, "") != nil {
		t.Fatal("max 0 must prune nothing")
	}
}

func TestLatestPerBranch(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := LatestPerBranch([]Entry{
		{PreviewApp: "a", Branch: "x", UpdatedAt: base},
		{PreviewApp: "b", Branch: "x", UpdatedAt: base.Add(time.Hour)},
	})
	if len(got) != 1 || got[0].PreviewApp != "b" {
		t.Fatalf("got %+v", got)
	}
}
