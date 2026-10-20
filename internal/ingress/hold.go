package ingress

import (
	"sync"
	"time"
)

// HoldTracker remembers when each host last had a live backend, so a host that
// loses its backend mid-deploy keeps answering a friendly 503 (and keeps its
// certificate) for a bounded window instead of vanishing from the config.
// It must outlive per-pass controllers.
type HoldTracker struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[string]time.Time
}

// NewHoldTracker builds a tracker; a window of zero or less disables holding.
func NewHoldTracker(window time.Duration) *HoldTracker {
	return &HoldTracker{window: window, seen: make(map[string]time.Time)}
}

// Routed records that host had a live backend at now.
func (t *HoldTracker) Routed(host string, now time.Time) {
	if t == nil || t.window <= 0 {
		return
	}
	t.mu.Lock()
	t.seen[host] = now
	t.mu.Unlock()
}

// Held reports whether host had a backend within the window, and forgets hosts
// whose window has lapsed.
func (t *HoldTracker) Held(host string, now time.Time) bool {
	if t == nil || t.window <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.seen[host]
	if !ok {
		return false
	}
	if now.Sub(at) > t.window {
		delete(t.seen, host)
		return false
	}
	return true
}
