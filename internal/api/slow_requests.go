package api

import (
	"sync"
	"time"
)

// slowRequestRingSize bounds memory: only the latest slow requests matter for triage.
const slowRequestRingSize = 50

// slowRequestEntry is one request that crossed the slow threshold.
type slowRequestEntry struct {
	At         time.Time `json:"at"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	Status     int       `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	Breakdown  string    `json:"breakdown,omitempty"`
}

// slowRequestRing keeps the most recent slow requests; the zero value is ready.
type slowRequestRing struct {
	mu      sync.Mutex
	entries []slowRequestEntry
	next    int
}

func (r *slowRequestRing) add(e slowRequestEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) < slowRequestRingSize {
		r.entries = append(r.entries, e)
		return
	}
	r.entries[r.next] = e
	r.next = (r.next + 1) % slowRequestRingSize
}

// snapshot returns the entries newest first.
func (r *slowRequestRing) snapshot() []slowRequestEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]slowRequestEntry, 0, len(r.entries))
	n := len(r.entries)
	for i := 1; i <= n; i++ {
		out = append(out, r.entries[((r.next-i)%n+n)%n])
	}
	return out
}
