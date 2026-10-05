package application

import (
	"sync"
	"time"
)

const reasonCrashLoopBackOff = "CrashLoopBackOff"

type backoffEntry struct {
	restarts  int
	next      time.Time
	lastStart time.Time
}

// RestartBackoff delays restarting a container that keeps exiting, so a
// crashlooping app cannot spin the reconciler (every die event triggers a
// pass) and the Docker daemon. In memory only and level-triggered: each
// pass re-derives whether a restart is allowed yet from the clock. One
// instance is shared by every controller, see WithRestartBackoff.
type RestartBackoff struct {
	mu         sync.Mutex
	entries    map[string]*backoffEntry
	base       time.Duration
	max        time.Duration
	resetAfter time.Duration
}

// NewRestartBackoff builds a tracker whose delay doubles from base per
// consecutive restart up to max. A container that stays up for resetAfter
// starts over. A zero base disables backoff.
func NewRestartBackoff(base, ceiling, resetAfter time.Duration) *RestartBackoff {
	return &RestartBackoff{entries: make(map[string]*backoffEntry), base: base, max: ceiling, resetAfter: resetAfter}
}

// remaining is how long name must still wait before a restart is allowed.
func (b *RestartBackoff) remaining(name string, now time.Time) (time.Duration, int) {
	if b == nil || b.base <= 0 {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[name]
	if !ok || !now.Before(e.next) {
		return 0, 0
	}
	return e.next.Sub(now), e.restarts
}

// recordRestart notes that name was just restarted and schedules the
// earliest time of the next restart.
func (b *RestartBackoff) recordRestart(name string, now time.Time) {
	if b == nil || b.base <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[name]
	if ok && now.Sub(e.lastStart) >= b.resetAfter {
		ok = false
	}
	if !ok {
		e = &backoffEntry{}
		b.entries[name] = e
	}
	e.restarts++
	delay := b.base
	for i := 1; i < e.restarts && delay < b.max; i++ {
		delay *= 2
	}
	if b.max > 0 && delay > b.max {
		delay = b.max
	}
	e.lastStart = now
	e.next = now.Add(delay)
}

// observeRunning forgets name once it has stayed up for resetAfter.
func (b *RestartBackoff) observeRunning(name string, now time.Time) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e, ok := b.entries[name]; ok && now.Sub(e.lastStart) >= b.resetAfter {
		delete(b.entries, name)
	}
}

// retain drops serviceName's entries for every container not in names.
func (b *RestartBackoff) retain(serviceName string, names []string) {
	if b == nil {
		return
	}
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for name := range b.entries {
		if !keep[name] && ownsContainer(serviceName, name) {
			delete(b.entries, name)
		}
	}
}
