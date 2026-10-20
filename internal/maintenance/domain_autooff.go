package maintenance

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ResolveAutoOff returns when open ended maintenance switches itself off.
// A requested duration of nil takes def; zero means no limit. A dated end
// (until set) needs no cut-off, its window already ends it.
func ResolveAutoOff(now time.Time, until *time.Time, requested *time.Duration, def time.Duration) (time.Time, error) {
	if until != nil {
		return time.Time{}, nil
	}
	d := def
	if requested != nil {
		d = *requested
	}
	switch {
	case d < 0:
		return time.Time{}, fmt.Errorf("max_duration_seconds must not be negative")
	case d > MaxDuration:
		return time.Time{}, fmt.Errorf("max_duration_seconds may be at most %s", MaxDuration)
	case d == 0:
		return time.Time{}, nil
	}
	return now.Add(d), nil
}

// KeepStart carries the original start across a re-save, so toggling the
// settings twice does not reset how long the site has been down.
func KeepStart(prev store.DomainMaintenance, wasOn bool, now time.Time) time.Time {
	if wasOn && !prev.StartedAt.IsZero() {
		return prev.StartedAt
	}
	return now
}

// Elapsed is how long m has been on at now, never negative so a clock that
// stepped back cannot show a future start.
func Elapsed(startedAt, now time.Time) time.Duration {
	if startedAt.IsZero() || now.Before(startedAt) {
		return 0
	}
	return now.Sub(startedAt)
}

// Overdue returns the maintenance rows that have been on longer than
// threshold. A zero threshold turns the check off.
func Overdue(active []store.DomainMaintenance, now time.Time, threshold time.Duration) []store.DomainMaintenance {
	if threshold <= 0 {
		return nil
	}
	var out []store.DomainMaintenance
	for _, m := range active {
		if Elapsed(m.StartedAt, now) > threshold {
			out = append(out, m)
		}
	}
	return out
}
