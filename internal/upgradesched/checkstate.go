package upgradesched

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

const (
	checkBackoffBase       = time.Minute
	defaultCheckBackoffMax = 6 * time.Hour
	defaultCheckAlertAfter = 5
	checkStateIOTimeout    = 5 * time.Second
	// SourceAPI is the releases API.
	SourceAPI = "api"
	// SourceFallback is the public release pages, which carry no notes.
	SourceFallback = "fallback"
)

// CheckStateStore persists the lookup health across restarts. A Store that
// does not implement it keeps the state in memory only.
type CheckStateStore interface {
	GetUpgradeCheckState(ctx context.Context) (store.UpgradeCheckState, error)
	SaveUpgradeCheckState(ctx context.Context, s store.UpgradeCheckState) error
}

// CheckStatus is what the scheduler knows about its own release lookups.
type CheckStatus struct {
	LastCheckAt time.Time
	LastOK      bool
	LastError   string
	Failures    int
	NextCheckAt time.Time
	// LastGoodAt is when a lookup last returned a fresh release list; a later
	// failure leaves it, so a stale list shows how old it is.
	LastGoodAt time.Time
	// Source is SourceAPI or SourceFallback for the last good lookup.
	Source string
}

// CheckStatus returns the state of the latest release lookups, zero before the first.
func (s *Scheduler) CheckStatus() CheckStatus {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	s.loadCheckLocked(context.Background())
	return s.check
}

func checkSource(c Candidate) string {
	if c.FromFallback {
		return SourceFallback
	}
	return SourceAPI
}

// loadCheckLocked reads the saved state once, so a restart resumes the streak
// and the backoff. checkMu must be held.
func (s *Scheduler) loadCheckLocked(ctx context.Context) {
	if s.checkLoaded {
		return
	}
	s.checkLoaded = true
	ss, ok := s.Store.(CheckStateStore)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, checkStateIOTimeout)
	defer cancel()
	row, err := ss.GetUpgradeCheckState(ctx)
	if err != nil {
		s.log().Warn("upgrade scheduler: load check state failed", "error", err.Error())
		return
	}
	s.check = CheckStatus{
		LastCheckAt: parseStamp(row.LastCheckAt), LastOK: row.LastOK, LastError: row.LastError, Failures: row.Failures,
		NextCheckAt: parseStamp(row.NextCheckAt), LastGoodAt: parseStamp(row.LastGoodAt), Source: row.LastSource,
	}
}

func (s *Scheduler) saveCheck(ctx context.Context, c CheckStatus) {
	ss, ok := s.Store.(CheckStateStore)
	if !ok {
		return
	}
	row := store.UpgradeCheckState{LastOK: c.LastOK, LastError: c.LastError, Failures: c.Failures, LastSource: c.Source}
	for dst, t := range map[*string]time.Time{&row.LastCheckAt: c.LastCheckAt, &row.NextCheckAt: c.NextCheckAt, &row.LastGoodAt: c.LastGoodAt} {
		if !t.IsZero() {
			*dst = stamp(t)
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkStateIOTimeout)
	defer cancel()
	if err := ss.SaveUpgradeCheckState(ctx, row); err != nil {
		s.log().Warn("upgrade scheduler: save check state failed", "error", err.Error())
	}
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
	s.loadCheckLocked(context.Background())
	return s.check.NextCheckAt.IsZero() || !s.now().Before(s.check.NextCheckAt)
}

// noteCheck records a lookup result, schedules the next try and raises one
// alert per failure streak plus one when lookups recover. source says where a
// good result came from.
func (s *Scheduler) noteCheck(ctx context.Context, err error, source string) {
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
	s.loadCheckLocked(ctx)
	prev := s.check
	if err == nil {
		s.check = CheckStatus{LastCheckAt: now, LastOK: true, LastGoodAt: now, Source: source}
		saved := s.check
		s.checkMu.Unlock()
		s.saveCheck(ctx, saved)
		if prev.Failures >= after {
			s.notify(ctx, "Automatic update checks work again.")
		}
		return
	}
	s.check.LastCheckAt = now
	s.check.LastOK, s.check.LastError = false, err.Error()
	s.check.Failures++
	s.check.NextCheckAt = now.Add(BackoffDelay(s.check.Failures, ceiling))
	failures, next, saved := s.check.Failures, s.check.NextCheckAt, s.check
	s.checkMu.Unlock()
	s.saveCheck(ctx, saved)
	s.log().Warn("upgrade scheduler: release check failed", "failures", failures, "next_try", stamp(next), "error", err.Error())
	if failures == after {
		msg := "Automatic updates could not look for a new release %d times in a row (%s). Updates are not applied until this works. Check that this host can reach GitHub, or set a fallback download URL."
		if errors.Is(err, ErrStaleList) {
			msg = "Automatic updates could only use an old release list %d times in a row (%s). Check that this host can reach GitHub, or set a fallback download URL."
		}
		s.notify(ctx, fmt.Sprintf(msg, failures, err.Error()))
	}
}
