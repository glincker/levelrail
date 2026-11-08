package alerting

import "time"

// ObserveOOM records one kernel OOM kill of a container owned by resourceID.
func (t *RestartTracker) ObserveOOM(resourceID string, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ooms[resourceID] = append(t.ooms[resourceID], at)
}

// OOMKillsSince counts resourceID's OOM kills strictly after since.
func (t *RestartTracker) OOMKillsSince(resourceID string, since time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := 0
	for _, ts := range t.ooms[resourceID] {
		if ts.After(since) {
			count++
		}
	}
	return count
}

// pruneTimes drops entries at or before olderThan and removes emptied keys.
func pruneTimes(m map[string][]time.Time, olderThan time.Time) {
	for id, times := range m {
		kept := times[:0]
		for _, ts := range times {
			if ts.After(olderThan) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(m, id)
		} else {
			m[id] = kept
		}
	}
}

// EvaluateOOMKilled fires while the app was OOM-killed at least
// RestartCountThreshold times inside RestartWindow; the window is the debounce.
func EvaluateOOMKilled(tracker *RestartTracker, r Rule, now time.Time) Rule {
	count := tracker.OOMKillsSince(r.ResourceID, now.Add(-r.RestartWindow))
	next := r
	next.LastEvaluatedAt = &now
	v := float64(count)
	next.LastValue = &v
	return advanceState(next, r, count >= r.RestartCountThreshold, 0, now)
}
