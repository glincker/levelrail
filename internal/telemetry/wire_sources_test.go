package telemetry

import (
	"testing"
	"time"
)

func TestLiveSourcesFollowSettings(t *testing.T) {
	t.Cleanup(func() {
		capTargetSource.Store(nil)
		cumulativeLookbackSource.Store(nil)
	})
	pct, idle := 70, time.Minute
	SetCapTargetPercentSource(func() int { return pct })
	SetCumulativeLookbackSource(func() time.Duration { return CumulativeLookback(idle, 15*time.Second) })
	if got := currentCapTargetPercent(); got != 70 {
		t.Fatalf("cap target %d, want 70", got)
	}
	pct, idle = 90, 10*time.Minute
	if got := currentCapTargetPercent(); got != 90 {
		t.Fatalf("cap target after change %d, want 90", got)
	}
	if got := cumulativeLookbackSeconds(); got != 1200 {
		t.Fatalf("lookback %d, want 1200", got)
	}
}
