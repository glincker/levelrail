package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func TestCostBudgetRoundTripAndEnforcementMerge(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for _, n := range []string{"web", "api"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: n, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := db.SaveCostBudget(ctx, costbudget.Budget{Name: "prod", Scope: costbudget.ScopeTag, ScopeValue: "prod", Monthly: 50, Action: spendcap.ActionThrottle}.Normalize(), now)
	if err != nil || b.ID == "" {
		t.Fatalf("save = %+v err = %v", b, err)
	}
	if _, err := db.SaveCostBudget(ctx, costbudget.Budget{Name: "prod", Scope: costbudget.ScopeApp, ScopeValue: "web", Monthly: 5}.Normalize(), now); !errors.Is(err, ErrCostBudgetNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	got, err := db.GetCostBudget(ctx, b.ID)
	if err != nil || got.ThrottleRPS != spendcap.DefaultThrottleRPS || len(got.Thresholds) != 3 {
		t.Fatalf("get = %+v err = %v", got, err)
	}

	st := costbudget.State{BudgetID: b.ID, Period: "2026-10", Highest: 100, Pct: 120, Enforced: spendcap.ActionThrottle, EnforcedApps: []string{"api", "web"}, UpdatedAt: now}
	if err := db.SaveCostBudgetState(ctx, st); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := db.GetCostBudgetState(ctx, b.ID)
	if err != nil || !ok || loaded.Highest != 100 || len(loaded.EnforcedApps) != 2 {
		t.Fatalf("state = %+v ok = %v err = %v", loaded, ok, err)
	}

	cp := spendcap.Cap{App: "web", MonthlyCostCeiling: 10, Action: spendcap.ActionPause}.Normalize()
	if err := db.SaveSpendCap(ctx, cp, spendcap.Event{App: "web", Period: "2026-10", Kind: spendcap.EventConfigured, At: now}, now); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSpendCapState(ctx, spendcap.State{App: "web", Period: "2026-10", Enforced: spendcap.ActionPause, Highest: 100, UpdatedAt: now}, nil); err != nil {
		t.Fatal(err)
	}
	enf, err := db.ListSpendCapEnforcements(ctx, "2026-10")
	if err != nil || len(enf) != 2 || enf[0].App != "api" || enf[0].Action != spendcap.ActionThrottle || enf[1].App != "web" || enf[1].Action != spendcap.ActionPause {
		t.Fatalf("pause must beat throttle, one entry per app: %+v err = %v", enf, err)
	}
	if enf, _ := db.ListSpendCapEnforcements(ctx, "2026-11"); len(enf) != 0 {
		t.Fatalf("a state from an earlier month must enforce nothing: %+v", enf)
	}

	if err := db.DeleteCostBudget(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetCostBudgetState(ctx, b.ID); ok {
		t.Fatal("state should cascade with the budget")
	}
	if err := db.DeleteCostBudget(ctx, b.ID); !errors.Is(err, costbudget.ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
