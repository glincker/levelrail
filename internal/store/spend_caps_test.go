package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func TestSpendCapRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSpendCap(ctx, "web"); !errors.Is(err, spendcap.ErrNotFound) {
		t.Fatalf("missing cap = %v", err)
	}

	cp := spendcap.Cap{App: "web", MonthlyEgressBytes: 1 << 30, MonthlyCostCeiling: 12.5, Action: spendcap.ActionThrottle}.Normalize()
	if err := db.SaveSpendCap(ctx, cp, spendcap.Event{App: "web", Period: "2026-10", Kind: spendcap.EventConfigured, At: now}, now); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSpendCap(ctx, "web")
	if err != nil || got.MonthlyEgressBytes != 1<<30 || got.MonthlyCostCeiling != 12.5 || got.ThrottleRPS != spendcap.DefaultThrottleRPS || len(got.Thresholds) != 3 {
		t.Fatalf("cap = %+v err=%v", got, err)
	}

	st := spendcap.State{App: "web", Period: "2026-10", Enforced: spendcap.ActionThrottle, Highest: 100, Pct: 101, UpdatedAt: now}
	ev := []spendcap.Event{{App: "web", Period: "2026-10", Kind: spendcap.EventEnforced, Detail: "throttle applied", At: now}}
	if err := db.SaveSpendCapState(ctx, st, ev); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := db.GetSpendCapState(ctx, "web")
	if err != nil || !ok || loaded.Enforced != spendcap.ActionThrottle || loaded.Highest != 100 {
		t.Fatalf("state = %+v ok=%v err=%v", loaded, ok, err)
	}

	enf, err := db.ListSpendCapEnforcements(ctx, "2026-10")
	if err != nil || len(enf) != 1 || enf[0].App != "web" || enf[0].ThrottleRPS != spendcap.DefaultThrottleRPS {
		t.Fatalf("enforcements = %+v err=%v", enf, err)
	}
	if enf, _ = db.ListSpendCapEnforcements(ctx, "2026-11"); len(enf) != 0 {
		t.Errorf("last month's state must not enforce: %+v", enf)
	}

	events, err := db.ListSpendCapEvents(ctx, "web", 10)
	if err != nil || len(events) != 2 || events[0].Kind != spendcap.EventEnforced {
		t.Fatalf("events = %+v err=%v", events, err)
	}

	if err := db.DeleteSpendCap(ctx, "web", spendcap.Event{App: "web", Period: "2026-10", Kind: spendcap.EventCleared, At: now}); err != nil {
		t.Fatal(err)
	}
	if enf, _ = db.ListSpendCapEnforcements(ctx, "2026-10"); len(enf) != 0 {
		t.Errorf("clearing must stop enforcement: %+v", enf)
	}
	if err := db.DeleteSpendCap(ctx, "web", spendcap.Event{App: "web"}); !errors.Is(err, spendcap.ErrNotFound) {
		t.Errorf("second delete = %v", err)
	}
}

func TestSpendCapGuardModeAndOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	cp := spendcap.Cap{App: "web", MonthlyCostCeiling: 10, GuardMode: spendcap.GuardBlockDeploys}.Normalize()
	if err := db.SaveSpendCap(ctx, cp, spendcap.Event{App: "web", Period: "2026-10", Kind: spendcap.EventConfigured, At: now}, now); err != nil {
		t.Fatal(err)
	}
	if got, err := db.GetSpendCap(ctx, "web"); err != nil || got.GuardMode != spendcap.GuardBlockDeploys {
		t.Fatalf("guard mode = %+v err=%v", got, err)
	}

	if _, ok, err := db.ActiveSpendOverride(ctx, "web", now); err != nil || ok {
		t.Fatalf("no override yet: ok=%v err=%v", ok, err)
	}
	o, err := spendcap.NewOverride("web", "hotfix for checkout", "root", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ev := spendcap.Event{App: "web", Period: "2026-10", Kind: spendcap.EventOverride, Detail: o.Reason, At: now}
	if _, err := db.SaveSpendOverride(ctx, o, ev); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := db.ActiveSpendOverride(ctx, "web", now.Add(30*time.Minute)); err != nil || !ok || got.Reason != "hotfix for checkout" {
		t.Fatalf("active override = %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, _ := db.ActiveSpendOverride(ctx, "web", now.Add(2*time.Hour)); ok {
		t.Error("an expired override must not apply")
	}
}
