package secretprovider

import (
	"errors"
	"sync"
	"time"
)

// ErrRateLimited is returned when a provider is asked for more reads than the live limit allows.
var ErrRateLimited = errors.New("provider read rate limit reached, try again shortly")

// rateLimiter counts provider requests per one second window, per provider connection.
// The zero value is ready to use.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
}

type rateWindow struct {
	start time.Time
	used  int
}

// allow reports whether one more request fits under limit per second. limit <= 0 means no limit.
func (l *rateLimiter) allow(key string, limit int, now time.Time) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]rateWindow{}
	}
	w := l.windows[key]
	if now.Sub(w.start) >= time.Second {
		w = rateWindow{start: now}
	}
	if w.used >= limit {
		l.windows[key] = w
		return false
	}
	w.used++
	l.windows[key] = w
	return true
}
