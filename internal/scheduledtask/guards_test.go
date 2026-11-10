package scheduledtask

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestGuardTimeoutAndRetentionAreClampedByLiveSettings(t *testing.T) {
	vals := map[string]int{KeyTimeoutMinSeconds: 5, KeyTimeoutMaxSeconds: 60, KeyRetentionMaxRuns: 10, KeyRetentionMaxDays: 7, KeyRetentionSweepMaxRows: 50}
	SetGuardResolver(func(k string) int { return vals[k] })
	t.Cleanup(func() { SetGuardResolver(func(string) int { return 0 }) })

	r := &Runner{}
	if got := r.attemptTimeout(store.ScheduledTask{TimeoutSeconds: 3600}); got != time.Minute {
		t.Errorf("long job timeout = %s, want clamped to 1m", got)
	}
	if got := r.attemptTimeout(store.ScheduledTask{TimeoutSeconds: 1}); got != 5*time.Second {
		t.Errorf("short job timeout = %s, want raised to 5s", got)
	}
	ended := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	p := r.retention(context.Background(), "st_1", ended)
	if p.Keep > 10 || p.MaxRows != 50 || !p.Before.Equal(ended.Add(-7*24*time.Hour)) {
		t.Errorf("retention = %+v, want keep <= 10, 50 rows per sweep, 7 day cutoff", p)
	}
}
