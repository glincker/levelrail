package telemetry

import (
	"sync"
	"sync/atomic"
	"time"
)

// Presence records the last time any viewer asked for live metrics; the API touches it, collectors read it.
type Presence struct {
	lastNanos atomic.Int64
	window    time.Duration
	now       func() time.Time

	// perResource tracks the last view per resource ID so one opened app returns to the fast cadence alone.
	mu          sync.Mutex
	perResource map[string]int64
}

// NewPresence builds a Presence where a touch counts as a viewer for window. The caller reads window from settings.
func NewPresence(window time.Duration) *Presence {
	return &Presence{window: window, now: time.Now}
}

// Touch marks a viewer as present now. Safe on a nil receiver.
func (p *Presence) Touch() {
	if p == nil {
		return
	}
	p.lastNanos.Store(p.now().UnixNano())
}

// Active reports whether a viewer touched within the window. A nil Presence is always active so unwired callers keep the fast cadence.
func (p *Presence) Active() bool {
	if p == nil {
		return true
	}
	last := p.lastNanos.Load()
	return last != 0 && p.now().Sub(time.Unix(0, last)) <= p.window
}

// TouchResource marks one resource (for example "service:web") as being viewed now. Safe on a nil receiver.
func (p *Presence) TouchResource(id string) {
	if p == nil || id == "" {
		return
	}
	n := p.now().UnixNano()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.perResource == nil {
		p.perResource = make(map[string]int64)
	}
	// Drop expired entries on write so the map stays bounded by recently viewed apps.
	for k, t := range p.perResource {
		if n-t > int64(p.window) {
			delete(p.perResource, k)
		}
	}
	p.perResource[id] = n
}

// ResourceActive reports whether id was viewed within the window. A nil Presence reports true.
func (p *Presence) ResourceActive(id string) bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	t, ok := p.perResource[id]
	p.mu.Unlock()
	return ok && p.now().UnixNano()-t <= int64(p.window)
}
