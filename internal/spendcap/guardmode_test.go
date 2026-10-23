package spendcap

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestParseWarnPercents(t *testing.T) {
	fallback := []int{50, 80, 100}
	tests := []struct {
		in   string
		want []int
	}{
		{"", fallback},
		{"80,100", []int{80, 100}},
		{" 100 , 80 ", []int{80, 100}},
		{"80,abc", fallback},
		{"0,50", fallback},
		{"101", fallback},
		{"1,2,3,4,5,6,7,8,9", fallback},
	}
	for _, tc := range tests {
		if got := ParseWarnPercents(tc.in, fallback); !slices.Equal(got, tc.want) {
			t.Errorf("ParseWarnPercents(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestGuardModeValidation(t *testing.T) {
	base := Cap{App: "web", MonthlyCostCeiling: 10}
	if c := base.Normalize(); c.GuardMode != GuardAlert || c.Validate() != nil {
		t.Fatalf("default guard mode = %q", c.GuardMode)
	}
	blocked := base
	blocked.GuardMode = GuardBlockDeploys
	if err := blocked.Normalize().Validate(); err != nil {
		t.Errorf("block_deploys = %v", err)
	}
	bad := base
	bad.GuardMode = "stop"
	if err := bad.Normalize().Validate(); err == nil {
		t.Error("an unknown guard mode must be rejected")
	}
}

func TestThresholdCrossings(t *testing.T) {
	c := Cap{App: "web", MonthlyCostCeiling: 100, Thresholds: []int{80, 100}, GuardMode: GuardBlockDeploys}.Normalize()
	tests := []struct {
		name      string
		cost      float64
		wantCross []int
		wantHigh  int
		reason    string
		healthy   bool
	}{
		{"below first level", 79.9, nil, 0, ReasonWithinBudget, true},
		{"exactly at warn", 80, []int{80}, 80, ReasonThresholdReached, true},
		{"between levels", 99, []int{80}, 80, ReasonThresholdReached, true},
		{"exactly at limit", 100, []int{80, 100}, 100, ReasonDeploysBlocked, false},
		{"far over", 250, []int{80, 100}, 100, ReasonDeploysBlocked, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Evaluate(c, Usage{Cost: tc.cost})
			if !slices.Equal(e.Crossed, tc.wantCross) || e.Highest() != tc.wantHigh {
				t.Fatalf("eval = %+v", e)
			}
			cond := Conditions(c, e, "", "")
			if cond.Reason != tc.reason || cond.Healthy != tc.healthy {
				t.Fatalf("condition = %+v", cond)
			}
			if e.Desired != ActionNone {
				t.Errorf("block_deploys must not enforce a hard action, got %q", e.Desired)
			}
		})
	}
}

func TestBlockDeploysTickIsIdempotentAndAlertsOncePerLevel(t *testing.T) {
	st := newFakeStore(Cap{App: "web", MonthlyCostCeiling: 100, Thresholds: []int{80, 100}, GuardMode: GuardBlockDeploys})
	u := &fakeUsage{use: map[string]Usage{"web": {Cost: 85}}}
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	var nudges int
	var nudgeErr error
	c := newController(st, u, &now, &nudges, &nudgeErr)
	ctx := context.Background()
	count := func(kind string) int {
		n := 0
		for _, e := range st.events {
			if e.Kind == kind {
				n++
			}
		}
		return n
	}

	for range 3 {
		if err := c.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count(EventThreshold) != 1 || st.states["web"].Highest != 80 {
		t.Fatalf("re-evaluating must not repeat the 80%% alert: %v", kinds(st.events))
	}

	u.use["web"] = Usage{Cost: 120}
	for range 3 {
		if err := c.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count(EventThreshold) != 2 || st.states["web"].Highest != 100 {
		t.Fatalf("each level alerts exactly once: %v", kinds(st.events))
	}
	if nudges != 0 {
		t.Errorf("block_deploys must not touch ingress, nudges = %d", nudges)
	}
	breaches, err := c.Breaches(ctx)
	if err != nil || len(breaches) != 1 {
		t.Fatalf("breaches = %+v err=%v", breaches, err)
	}
	again, _ := c.Breaches(ctx)
	if again[0].Key != breaches[0].Key {
		t.Errorf("the breach key must be stable so the alert does not repeat: %q vs %q", breaches[0].Key, again[0].Key)
	}

	now = time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC)
	u.use["web"] = Usage{Cost: 5}
	if err := c.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if st.states["web"].Period != "2026-11" || st.states["web"].Highest != 0 {
		t.Fatalf("rollover must reset the levels: %+v", st.states["web"])
	}
	if b, _ := c.Breaches(ctx); len(b) != 0 {
		t.Errorf("a new month starts with no breach: %+v", b)
	}
}

func TestNewOverride(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if _, err := NewOverride("web", "  ", "root", now, time.Hour); err == nil {
		t.Error("an override needs a reason")
	}
	o, err := NewOverride("web", " hotfix ", "root", now, time.Hour)
	if err != nil || o.Reason != "hotfix" || !o.Active(now.Add(59*time.Minute)) || o.Active(now.Add(time.Hour)) {
		t.Fatalf("override = %+v err=%v", o, err)
	}
	t.Setenv(EnvOverrideTTL, "30m")
	if OverrideTTL() != 30*time.Minute {
		t.Errorf("ttl from env = %v", OverrideTTL())
	}
	t.Setenv(EnvOverrideTTL, "nonsense")
	if OverrideTTL() != time.Hour {
		t.Errorf("bad ttl must fall back, got %v", OverrideTTL())
	}
}

func TestPreviewPause(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	p := PreviewPause([]AppLoad{
		{Name: "b", Replicas: 2, RecentDaily: []float64{2, 4}},
		{Name: "a", Replicas: 1, MonthToDate: 10},
		{Name: "idle", Replicas: 1},
	}, now)
	if !p.DryRun || p.TotalReplicas != 4 || len(p.Apps) != 3 || p.Apps[0].Name != "a" {
		t.Fatalf("preview = %+v", p)
	}
	if p.RemainingDays != 21 {
		t.Fatalf("remaining days = %v", p.RemainingDays)
	}
	if p.Apps[0].DailyRunRate != 1 || p.Apps[0].EstimatedSaving != 21 {
		t.Errorf("month to date fallback = %+v", p.Apps[0])
	}
	if p.Apps[1].DailyRunRate != 3 || p.Apps[1].EstimatedSaving != 63 {
		t.Errorf("recent average = %+v", p.Apps[1])
	}
	if p.Apps[2].EstimatedSaving != 0 || p.EstimatedSaving != 84 {
		t.Errorf("totals = %+v", p)
	}
}
