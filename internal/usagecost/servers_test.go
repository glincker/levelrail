package usagecost

import (
	"math"
	"testing"
)

func TestServerSharesTwoApps(t *testing.T) {
	gb := float64(gib)
	rows := []DailyCost{
		{DailyUsage: DailyUsage{Day: "2026-11-01", Kind: KindApp, Name: "a", NodeID: "n1", UpSeconds: secondsPerDay, MemAvgBytes: 3 * gb}},
		{DailyUsage: DailyUsage{Day: "2026-11-01", Kind: KindApp, Name: "b", NodeID: "n1", UpSeconds: secondsPerDay, MemAvgBytes: gb}},
		{DailyUsage: DailyUsage{Day: "2026-11-01", Kind: KindApp, Name: "c", NodeID: "n2", UpSeconds: secondsPerDay, MemAvgBytes: gb}},
	}
	got := ServerShares(ServerSharesInput{
		Rows:     rows,
		Nodes:    map[string]NodePrice{"n1": {MonthlyPrice: 10, MemoryGiB: 8}},
		Settings: Settings{CPUWeightPct: 100},
		Savings:  map[string]float64{"app:a": 1.5},
	})
	if len(got) != 2 {
		t.Fatalf("servers: %d", len(got))
	}
	n1 := got[0]
	if math.Abs(*n1.Apps[0].Share-3.75) > 1e-9 || math.Abs(*n1.Apps[1].Share-1.25) > 1e-9 || math.Abs(*n1.Idle-5) > 1e-9 {
		t.Fatalf("shares %v %v idle %v", *n1.Apps[0].Share, *n1.Apps[1].Share, *n1.Idle)
	}
	if n1.Apps[0].Saving == nil || *n1.Apps[0].Saving != 1.5 || n1.Apps[1].Saving != nil {
		t.Fatalf("savings wrong")
	}
	if got[1].MonthlyPrice != nil || got[1].Apps[0].Share != nil || got[1].Idle != nil {
		t.Fatalf("unpriced node must be null")
	}
}
