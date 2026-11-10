package iac

import "testing"

func TestIaCGuardsFollowLiveSource(t *testing.T) {
	SetPlanValueSource(func() int { return 33 })
	SetPruneGuardSource(func() PruneGuard { return PruneGuard{MaxPercent: 7, MaxCount: 2} })
	t.Cleanup(func() { planValueLive.Store(nil); pruneGuardLive.Store(nil) })
	if got := PlanValueMax(); got != 33 {
		t.Errorf("PlanValueMax = %d, want 33", got)
	}
	if g := PruneGuardFromEnv(); g.MaxPercent != 7 || g.MaxCount != 2 {
		t.Errorf("guard = %+v", g)
	}
}
