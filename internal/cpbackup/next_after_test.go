package cpbackup

import (
	"testing"
	"time"
)

func TestNextAfter_NeverRunsIsNilNotYearOne(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	if got := nextAfter("0 0 30 2 *", time.Time{}, now); got != nil {
		t.Fatalf("impossible date: got %v, want nil", got)
	}
	if got := nextAfter("0 3 * * *", time.Time{}, now); got == nil || got.IsZero() {
		t.Fatalf("daily: got %v, want a real time", got)
	}
	if overdue("0 0 30 2 *", now.Add(-time.Hour), now, time.Minute) {
		t.Fatal("a schedule that never runs must not be overdue")
	}
}
