package alerting

import (
	"testing"
	"time"
)

func TestOOMKilledFiresInsideWindowOnly(t *testing.T) {
	now := time.Now()
	tr := NewRestartTracker()
	tr.ObserveOOM("service:web", now.Add(-20*time.Minute))
	tr.ObserveOOM("service:web", now.Add(-2*time.Minute))
	tr.ObserveOOM("service:other", now.Add(-time.Minute))
	rule := Rule{Kind: KindOOMKilled, ResourceID: "service:web", RestartCountThreshold: 2, RestartWindow: 10 * time.Minute}

	if got := EvaluateOOMKilled(tr, rule, now); got.Firing || *got.LastValue != 1 {
		t.Fatalf("one kill in window must not fire: %+v", got)
	}
	tr.ObserveOOM("service:web", now.Add(-time.Minute))
	got := EvaluateOOMKilled(tr, rule, now)
	if *got.LastValue != 2 {
		t.Fatalf("count = %v, want 2", *got.LastValue)
	}
	tr.Prune(now.Add(-5 * time.Minute))
	if tr.OOMKillsSince("service:web", now.Add(-time.Hour)) != 2 {
		t.Fatal("prune must keep recent kills only")
	}
}
