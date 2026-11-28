package upgradesched

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		failures int
		max      time.Duration
		want     time.Duration
	}{
		{1, time.Hour, time.Minute},
		{2, time.Hour, 2 * time.Minute},
		{4, time.Hour, 8 * time.Minute},
		{10, time.Hour, time.Hour},
		{3, 0, 4 * time.Minute},
		{40, 0, 6 * time.Hour},
	}
	for _, tt := range tests {
		if got := BackoffDelay(tt.failures, tt.max); got != tt.want {
			t.Errorf("BackoffDelay(%d, %s) = %s, want %s", tt.failures, tt.max, got, tt.want)
		}
	}
}

func TestFailedChecksBackOffAlertOnceAndRecover(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("any"))
	h.host.latestErr = errors.New("github unreachable")
	s := h.sched()
	s.CheckAlertAfter = func(context.Context) int { return 3 }
	lookups := 0
	h.host.onLatest = func() { lookups++ }

	h.tick(s)
	h.tick(s) // inside the backoff: no second lookup
	if lookups != 1 {
		t.Fatalf("lookups inside the backoff = %d, want 1", lookups)
	}
	if st := s.CheckStatus(); st.Failures != 1 || st.LastOK || st.NextCheckAt.Sub(h.now) != time.Minute {
		t.Fatalf("status after one failure = %+v", st)
	}
	for _, step := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute} {
		h.now = h.now.Add(step)
		h.tick(s)
	}
	alerts := 0
	for _, n := range h.notes.texts {
		if strings.Contains(n, "could not look for a new release") {
			alerts++
		}
	}
	if alerts != 1 {
		t.Fatalf("alerts = %d, want exactly 1 per streak: %v", alerts, h.notes.texts)
	}

	h.host.latestErr = nil
	h.now = h.now.Add(time.Hour)
	h.tick(s)
	if st := s.CheckStatus(); st.Failures != 0 || !st.LastOK {
		t.Fatalf("status after recovery = %+v", st)
	}
	last := h.notes.texts[len(h.notes.texts)-1]
	if !strings.Contains(last, "work again") && h.active() == nil {
		t.Fatalf("expected a recovery notice or a schedule, got %v", h.notes.texts)
	}
}

func TestAutomaticUpgradeNeverRunsWithoutABackup(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		wantApplied int
	}{
		{"automatic is blocked", store.UpgradeSourceAuto, 0},
		{"manual keeps its preflight verdict", store.UpgradeSourceManual, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.host.backupErr = ErrBackupNotAvail
			s := h.sched()
			if tt.source == store.UpgradeSourceAuto {
				h.setPolicy(autoPolicy("any"))
				h.tick(s)
				h.now = time.Date(2026, 10, 12, 3, 0, 5, 0, time.UTC)
			} else {
				h.schedule(s, h.now.Add(time.Hour))
				h.now = h.now.Add(2 * time.Hour)
			}
			h.tick(s)
			if len(h.host.applied) != tt.wantApplied {
				t.Fatalf("applied = %v, want %d", h.host.applied, tt.wantApplied)
			}
		})
	}
}
