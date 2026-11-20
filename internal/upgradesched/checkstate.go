package upgradesched

import (
	"context"
	"fmt"
	"time"
)

const (
	checkBackoffBase       = time.Minute
	defaultCheckBackoffMax = 6 * time.Hour
	defaultCheckAlertAfter = 5
)

// CheckStatus is what the scheduler knows about its own release lookups.
type CheckStatus struct {
	LastCheckAt time.Time
	LastOK      bool
	LastError   string
	Failures    int
	NextCheckAt time.Time
}

// CheckStatus returns the state of the latest release lookups, zero before the first.
func (s *Scheduler) CheckStatus() CheckStatus {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	return s.check
}

// BackoffDelay is the wait after the nth consecutive failed check: one minute,
// doubling, capped at max.
func BackoffDelay(failures int, ceiling time.Duration) time.Duration {
	if ceiling <= 0 {
		ceiling = defaultCheckBackoffMax
	}
	d := checkBackoffBase
	for i := 1; i < failures && d < ceiling; i++ {
		d *= 2
	}
	return min(d, ceiling)
}

// checkDue reports whether a lookup may run now, so a blocked source is not hammered every tick.
func (s *Scheduler) checkDue() bool {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	return s.check.NextCheckAt.IsZero() || !s.now().Before(s.check.NextCheckAt)
}

// noteCheck records a lookup result, schedules the next try and raises one
// alert per failure streak plus one when lookups recover.
func (s *Scheduler) noteCheck(ctx context.Context, err error) {
	ceiling, after := defaultCheckBackoffMax, defaultCheckAlertAfter
	if s.CheckBackoffMax != nil {
		if v := s.CheckBackoffMax(ctx); v > 0 {
			ceiling = v
		}
	}
	if s.CheckAlertAfter != nil {
		if v := s.CheckAlertAfter(ctx); v > 0 {
			after = v
		}
	}
	now := s.now()
	s.checkMu.Lock()
	prev := s.check
	s.check.LastCheckAt = now
	if err == nil {
		s.check = CheckStatus{LastCheckAt: now, LastOK: true}
		s.checkMu.Unlock()
		if prev.Failures >= after {
			s.notify(ctx, "Automatic update checks work again.")
		}
		return
	}
	s.check.LastOK, s.check.LastError = false, err.Error()
	s.check.Failures++
	s.check.NextCheckAt = now.Add(BackoffDelay(s.check.Failures, ceiling))
	failures, next := s.check.Failures, s.check.NextCheckAt
	s.checkMu.Unlock()
	s.log().Warn("upgrade scheduler: release check failed", "failures", failures, "next_try", stamp(next), "error", err.Error())
	if failures == after {
		s.notify(ctx, fmt.Sprintf("Automatic updates could not look for a new release %d times in a row (%s). Updates are not applied until this works. Check that this host can reach GitHub, or set a fallback download URL.", failures, err.Error()))
	}
}
