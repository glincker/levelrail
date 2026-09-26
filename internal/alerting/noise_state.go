package alerting

import (
	"fmt"
	"time"
)

// NoiseConfig holds the control plane defaults for alert noise control.
// A zero value disables the corresponding feature; per-rule fields on
// Rule override ConsecutiveFailures and the flap settings.
type NoiseConfig struct {
	ConsecutiveFailures int
	FlapThreshold       int
	FlapWindow          time.Duration
	GroupWindow         time.Duration
	RateLimit           int
	RateWindow          time.Duration
	HistoryRetention    time.Duration
}

// Defaults for NoiseConfig, overridable per environment in cmd/levelrail.
const (
	DefaultFlapThreshold    = 5
	DefaultFlapWindow       = 30 * time.Minute
	DefaultRateLimit        = 30
	DefaultRateWindow       = 10 * time.Minute
	DefaultHistoryRetention = 30 * 24 * time.Hour
)

// streakTracker counts consecutive evaluation ticks a rule's condition held.
type streakTracker struct {
	counts map[string]int
}

func newStreakTracker() *streakTracker { return &streakTracker{counts: map[string]int{}} }

// apply holds a rule that is about to start firing in its pending state
// until its condition has held for need consecutive ticks. A rule that is
// already firing is never affected.
func (s *streakTracker) apply(prev, next Rule, need int, now time.Time) Rule {
	condition := next.Firing || next.PendingSince != nil
	if !condition {
		delete(s.counts, next.ID)
		return next
	}
	if prev.Firing {
		return next
	}
	s.counts[next.ID]++
	if need <= 1 || s.counts[next.ID] >= need {
		return next
	}
	next.Firing = false
	next.FiringSince = nil
	if next.PendingSince == nil {
		next.PendingSince = &now
	}
	return next
}

// flapEntry is one rule currently marked flapping.
type flapEntry struct {
	threshold int
	window    time.Duration
	last      Event
}

// flapTracker marks a rule flapping when it fires more than threshold
// times inside window, and clears the mark once fires drop to half the
// threshold (hysteresis, so a rule hovering at the limit does not toggle).
type flapTracker struct {
	fires    map[string][]time.Time
	flapping map[string]*flapEntry
}

func newFlapTracker() *flapTracker {
	return &flapTracker{fires: map[string][]time.Time{}, flapping: map[string]*flapEntry{}}
}

func trimBefore(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return ts[i:]
}

func (f *flapTracker) observe(ev Event, threshold int, window time.Duration, now time.Time) (flapping, entered bool) {
	if threshold <= 0 || window <= 0 {
		return false, false
	}
	id := ev.Rule.ID
	if !ev.Resolved {
		f.fires[id] = append(f.fires[id], now)
	}
	f.fires[id] = trimBefore(f.fires[id], now.Add(-window))

	if e, ok := f.flapping[id]; ok {
		e.last = ev
		return true, false
	}
	if len(f.fires[id]) > threshold {
		f.flapping[id] = &flapEntry{threshold: threshold, window: window, last: ev}
		return true, true
	}
	return false, false
}

func (f *flapTracker) fireCount(id string) int { return len(f.fires[id]) }

// sweep returns rules that have calmed down and forgets idle history.
func (f *flapTracker) sweep(now time.Time) []Event {
	var ended []Event
	for id, e := range f.flapping {
		f.fires[id] = trimBefore(f.fires[id], now.Add(-e.window))
		if len(f.fires[id])*2 <= e.threshold {
			ended = append(ended, e.last)
			delete(f.flapping, id)
		}
	}
	for id, ts := range f.fires {
		if _, ok := f.flapping[id]; ok {
			continue
		}
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > DefaultFlapWindow*4 {
			delete(f.fires, id)
		}
	}
	return ended
}

// rateLimiter is a per-key sliding window: at most limit sends per window.
type rateLimiter struct {
	sends map[string][]time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{sends: map[string][]time.Time{}} }

func (r *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	if limit <= 0 || window <= 0 {
		return true
	}
	ts := trimBefore(r.sends[key], now.Add(-window))
	if len(ts) >= limit {
		r.sends[key] = ts
		return false
	}
	r.sends[key] = append(ts, now)
	return true
}

// alertGroup buffers firing alerts for one delivery target and resource
// until the group window closes.
type alertGroup struct {
	opened  time.Time
	order   []string
	members map[string]Event
}

type groupBuffer struct {
	groups map[string]*alertGroup
}

func newGroupBuffer() *groupBuffer { return &groupBuffer{groups: map[string]*alertGroup{}} }

// add buffers ev under key; a repeat of the same rule replaces its entry (deduplication).
func (g *groupBuffer) add(key string, ev Event, now time.Time) {
	grp, ok := g.groups[key]
	if !ok {
		grp = &alertGroup{opened: now, members: map[string]Event{}}
		g.groups[key] = grp
	}
	if _, dup := grp.members[ev.Rule.ID]; !dup {
		grp.order = append(grp.order, ev.Rule.ID)
	}
	grp.members[ev.Rule.ID] = ev
}

// remove drops a buffered fire for ruleID and reports whether one existed.
func (g *groupBuffer) remove(ruleID string) bool {
	for key, grp := range g.groups {
		if _, ok := grp.members[ruleID]; !ok {
			continue
		}
		delete(grp.members, ruleID)
		for i, id := range grp.order {
			if id == ruleID {
				grp.order = append(grp.order[:i], grp.order[i+1:]...)
				break
			}
		}
		if len(grp.members) == 0 {
			delete(g.groups, key)
		}
		return true
	}
	return false
}

// due removes and returns every group open for at least window, or all
// groups when window is zero (shutdown flush).
func (g *groupBuffer) due(window time.Duration, now time.Time) [][]Event {
	var out [][]Event
	for key, grp := range g.groups {
		if window > 0 && now.Sub(grp.opened) < window {
			continue
		}
		evs := make([]Event, 0, len(grp.order))
		for _, id := range grp.order {
			evs = append(evs, grp.members[id])
		}
		out = append(out, evs)
		delete(g.groups, key)
	}
	return out
}

// combineGroup folds a group of firing alerts into one event carrying a count.
func combineGroup(evs []Event) Event {
	first := evs[0]
	if len(evs) == 1 {
		return first
	}
	lines := make([]string, len(evs))
	for i, e := range evs {
		lines[i] = fmt.Sprintf("%s (%s, %s)", e.Rule.Name, e.Rule.Kind, severityOrDefault(e.Rule.Severity))
	}
	first.Headline = fmt.Sprintf("[FIRING] %d related alerts on %s", len(evs), first.Rule.ResourceID)
	first.GroupNotices = lines
	first.GroupCount = len(evs)
	return first
}
