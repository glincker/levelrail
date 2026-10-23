package spendguard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

type fakeStore struct {
	cap       *spendcap.Cap
	capState  *spendcap.State
	budgets   []costbudget.Budget
	bStates   map[string]costbudget.State
	override  *spendcap.Override
	failState bool
}

func (f *fakeStore) GetSpendCap(context.Context, string) (spendcap.Cap, error) {
	if f.cap == nil {
		return spendcap.Cap{}, spendcap.ErrNotFound
	}
	return *f.cap, nil
}

func (f *fakeStore) GetSpendCapState(context.Context, string) (spendcap.State, bool, error) {
	if f.failState {
		return spendcap.State{}, false, errors.New("disk")
	}
	if f.capState == nil {
		return spendcap.State{}, false, nil
	}
	return *f.capState, true, nil
}

func (f *fakeStore) ListCostBudgets(context.Context) ([]costbudget.Budget, error) {
	return f.budgets, nil
}

func (f *fakeStore) GetCostBudgetState(_ context.Context, id string) (costbudget.State, bool, error) {
	s, ok := f.bStates[id]
	return s, ok, nil
}

func (f *fakeStore) ActiveSpendOverride(_ context.Context, _ string, now time.Time) (spendcap.Override, bool, error) {
	if f.override != nil && f.override.Active(now) {
		return *f.override, true, nil
	}
	return spendcap.Override{}, false, nil
}

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func blockCap() *spendcap.Cap {
	return &spendcap.Cap{App: "web", MonthlyCostCeiling: 10, GuardMode: spendcap.GuardBlockDeploys}
}

func TestGuardCheck(t *testing.T) {
	over := &spendcap.State{App: "web", Period: "2026-10", Pct: 120}
	under := &spendcap.State{App: "web", Period: "2026-10", Pct: 60}
	lastMonth := &spendcap.State{App: "web", Period: "2026-09", Pct: 150}
	alertOnly := &spendcap.Cap{App: "web", MonthlyCostCeiling: 10, GuardMode: spendcap.GuardAlert}
	live := &spendcap.Override{Reason: "hotfix", ExpiresAt: now.Add(time.Hour)}
	expired := &spendcap.Override{Reason: "old", ExpiresAt: now.Add(-time.Minute)}
	proj := costbudget.Budget{ID: "b1", Name: "team-a", Scope: costbudget.ScopeProject, ScopeValue: "p1", GuardMode: spendcap.GuardBlockDeploys}
	otherProj := costbudget.Budget{ID: "b2", Name: "team-b", Scope: costbudget.ScopeProject, ScopeValue: "p2", GuardMode: spendcap.GuardBlockDeploys}

	tests := []struct {
		name        string
		st          fakeStore
		wantAllowed bool
		wantBlocked bool
		wantOver    bool
	}{
		{name: "no cap allows", st: fakeStore{}, wantAllowed: true},
		{name: "under limit allows", st: fakeStore{cap: blockCap(), capState: under}, wantAllowed: true},
		{name: "over limit refuses", st: fakeStore{cap: blockCap(), capState: over}, wantBlocked: true},
		{name: "alert mode never blocks", st: fakeStore{cap: alertOnly, capState: over}, wantAllowed: true},
		{name: "last month state is ignored after rollover", st: fakeStore{cap: blockCap(), capState: lastMonth}, wantAllowed: true},
		{name: "live override allows", st: fakeStore{cap: blockCap(), capState: over, override: live}, wantAllowed: true, wantBlocked: true, wantOver: true},
		{name: "expired override refuses", st: fakeStore{cap: blockCap(), capState: over, override: expired}, wantBlocked: true},
		{
			name:        "project budget over limit refuses",
			st:          fakeStore{budgets: []costbudget.Budget{proj}, bStates: map[string]costbudget.State{"b1": {Period: "2026-10", Pct: 101}}},
			wantBlocked: true,
		},
		{
			name:        "other project budget does not apply",
			st:          fakeStore{budgets: []costbudget.Budget{otherProj}, bStates: map[string]costbudget.State{"b2": {Period: "2026-10", Pct: 300}}},
			wantAllowed: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &Guard{Store: &tc.st, Now: func() time.Time { return now }, ProjectOf: func(context.Context, string) (string, error) { return "p1", nil }}
			got, err := g.Check(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}
			if got.Allowed != tc.wantAllowed || got.Blocked != tc.wantBlocked || (got.Override != nil) != tc.wantOver {
				t.Fatalf("status = %+v", got)
			}
			err = g.BeforeDeploy(context.Background(), "web")
			if tc.wantAllowed && err != nil {
				t.Fatalf("BeforeDeploy = %v", err)
			}
			if !tc.wantAllowed && !errors.Is(err, ErrDeploysBlocked) {
				t.Fatalf("BeforeDeploy = %v, want ErrDeploysBlocked", err)
			}
		})
	}
}

func TestGuardFailsOpenOnStoreError(t *testing.T) {
	g := &Guard{Store: &fakeStore{cap: blockCap(), failState: true}, Now: func() time.Time { return now }}
	if err := g.BeforeDeploy(context.Background(), "web"); err != nil {
		t.Fatalf("a storage error must not block a deploy: %v", err)
	}
}
