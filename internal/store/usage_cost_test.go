package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/usagecost"
)

func TestUsageCostSettingsAndNodesRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)

	got, err := db.GetUsageCostSettings(ctx)
	if err != nil || got.Mode != usagecost.ModeUnit {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	want := usagecost.Settings{Mode: usagecost.ModeNode, Currency: "EUR", IdleSpreadPct: 25, Basis: usagecost.BasisP50, CPUWeightPct: 60, CPUCoreHour: 0.03}
	if err := db.SaveUsageCostSettings(ctx, want, now); err != nil {
		t.Fatal(err)
	}
	if got, err = db.GetUsageCostSettings(ctx); err != nil || got != want {
		t.Fatalf("settings = %+v, %v, want %+v", got, err, want)
	}

	nodes := []usagecost.NodePrice{{NodeID: "n1", MonthlyPrice: 40, VCPUs: 4, MemoryGiB: 8}}
	if err := db.ReplaceUsageCostNodes(ctx, nodes, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceUsageCostNodes(ctx, nodes, now); err != nil {
		t.Fatal(err)
	}
	m, err := db.ListUsageCostNodes(ctx)
	if err != nil || len(m) != 1 || m["n1"].MonthlyPrice != 40 {
		t.Fatalf("nodes = %+v, %v", m, err)
	}
}

func TestUsageDaySnapshotReplacesAndSurvivesRange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	row := func(name string, cost float64) usagecost.DailyCost {
		return usagecost.DailyCost{DailyUsage: usagecost.DailyUsage{Day: "2026-10-05", Kind: usagecost.KindApp, Name: name, ProjectID: "p1"}, ComputeCost: cost}
	}
	if err := db.SaveUsageDay(ctx, "2026-10-05", []usagecost.DailyCost{row("a", 1), row("b", 2)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUsageDay(ctx, "2026-10-05", []usagecost.DailyCost{row("a", 5)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUsageDay(ctx, "2026-10-06", []usagecost.DailyCost{row("a", 7)}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListUsageDays(ctx, "2026-10-05", "2026-10-06")
	if err != nil || len(rows) != 1 || rows[0].ComputeCost != 5 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

func TestUsageBudgetUpsertPerScope(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	first, err := db.SaveUsageBudget(ctx, usagecost.Budget{Scope: usagecost.ScopeTeam, ScopeID: "t1", Monthly: 100, WarnPct: 80}, now)
	if err != nil || first.ID == "" {
		t.Fatalf("save = %+v, %v", first, err)
	}
	second, err := db.SaveUsageBudget(ctx, usagecost.Budget{Scope: usagecost.ScopeTeam, ScopeID: "t1", Monthly: 250, WarnPct: 90}, now)
	if err != nil || second.ID != first.ID {
		t.Fatalf("upsert changed id: %+v, %v", second, err)
	}
	got, err := db.GetUsageBudget(ctx, first.ID)
	if err != nil || got.Monthly != 250 || got.WarnPct != 90 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := db.DeleteUsageBudget(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUsageBudget(ctx, first.ID); !errors.Is(err, ErrUsageBudgetNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
