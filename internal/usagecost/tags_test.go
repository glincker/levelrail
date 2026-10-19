package usagecost

import (
	"testing"
	"time"
)

func TestRollupTags(t *testing.T) {
	p := Period{From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	prevP := Period{From: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), To: p.From}
	row := func(day, kind, name string, cost float64) DailyCost {
		return DailyCost{DailyUsage: DailyUsage{Day: day, Kind: kind, Name: name}, ComputeCost: cost}
	}
	rows := []DailyCost{
		row("2026-10-01", KindApp, "web", 4),
		row("2026-10-02", KindApp, "web", 4),
		row("2026-10-01", KindDatabase, "pg", 2),
		row("2026-10-01", KindApp, "solo", 1),
		row("2026-10-01", KindIdle, "", 9),
	}
	prev := []DailyCost{row("2026-09-30", KindApp, "web", 5)}
	tagsOf := map[string][]string{
		"app:web":     {"team:shop", "env:prod"},
		"database:pg": {"team:shop"},
	}
	got := RollupTags(p, prevP, rows, prev, tagsOf)
	want := map[string]float64{"team:shop": 10, "env:prod": 8, UntaggedKey: 1}
	if len(got) != len(want) {
		t.Fatalf("got %d groups, want %d: %+v", len(got), len(want), got)
	}
	for _, g := range got {
		if g.Total != want[g.Key] {
			t.Errorf("tag %s total = %v, want %v", g.Key, g.Total, want[g.Key])
		}
	}
	if got[0].Key != "team:shop" || got[0].Previous != 5 {
		t.Errorf("top group = %+v, want team:shop with previous 5", got[0])
	}
}
