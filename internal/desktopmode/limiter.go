package desktopmode

import (
	"sync"
	"time"
)

// limiter caps token attempts per minute and locks out after repeated bad
// tokens. State is global: the only legitimate caller is the one host app.
type limiter struct {
	mu          sync.Mutex
	rpm         int
	maxFailures int
	lockout     time.Duration
	now         func() time.Time
	window      []time.Time
	failures    int
	lockedUntil time.Time
}

// allow records an attempt and returns how long to wait when refused.
func (l *limiter) allow() (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.lockedUntil) {
		return false, l.lockedUntil.Sub(now)
	}
	cut := now.Add(-time.Minute)
	kept := l.window[:0]
	for _, t := range l.window {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.window = kept
	if l.rpm > 0 && len(l.window) >= l.rpm {
		return false, l.window[0].Add(time.Minute).Sub(now)
	}
	l.window = append(l.window, now)
	return true, 0
}

// fail counts a bad token and reports whether it tripped the lockout.
func (l *limiter) fail() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.maxFailures > 0 && l.failures >= l.maxFailures {
		l.lockedUntil = l.now().Add(l.lockout)
		l.failures = 0
		return true
	}
	return false
}

func (l *limiter) success() {
	l.mu.Lock()
	l.failures = 0
	l.mu.Unlock()
}
