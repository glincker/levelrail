package telemetry

import (
	"sync/atomic"
	"time"
)

// Presence records the last time any viewer asked for live metrics; the API touches it, collectors read it.
type Presence struct {
	lastNanos atomic.Int64
	window    time.Duration
	now       func() time.Time
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
