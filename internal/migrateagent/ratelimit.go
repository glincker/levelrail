package migrateagent

import (
	"net"
	"sync"
	"time"
)

// ipLimiter is a fixed-window counter per source address. Stale windows are
// swept on use so an address-spraying client cannot grow the map without bound.
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*ipWindow
	swept  time.Time
}

type ipWindow struct {
	start time.Time
	count int
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	return &ipLimiter{limit: limit, window: window, hits: map[string]*ipWindow{}}
}

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) >= l.window {
		for k, w := range l.hits {
			if now.Sub(w.start) >= l.window {
				delete(l.hits, k)
			}
		}
		l.swept = now
	}
	w := l.hits[ip]
	if w == nil || now.Sub(w.start) >= l.window {
		l.hits[ip] = &ipWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= l.limit
}

func hostOnly(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
