package edgefw

import (
	"net/netip"
	"sync"
	"time"
)

type rateEntry struct {
	start time.Time
	n     int
}

// rateLimiter counts requests per client IP in fixed windows. Entries are
// capped: past the cap an arbitrary entry is evicted, so a flood of distinct
// addresses cannot grow it.
type rateLimiter struct {
	mu      sync.Mutex
	count   int
	window  time.Duration
	cap     int
	entries map[netip.Addr]rateEntry
}

func newRateLimiter(count, windowSeconds, capacity int) *rateLimiter {
	return &rateLimiter{count: count, window: time.Duration(windowSeconds) * time.Second, cap: capacity, entries: make(map[netip.Addr]rateEntry)}
}

func (l *rateLimiter) same(count, windowSeconds int) bool {
	return l.count == count && l.window == time.Duration(windowSeconds)*time.Second
}

// allow records one request from ip and reports whether it is within the
// limit; when it is not, retryAfter is the seconds until the window resets.
func (l *rateLimiter) allow(ip netip.Addr, now time.Time) (ok bool, retryAfter int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, found := l.entries[ip]
	if !found || now.Sub(e.start) >= l.window {
		if !found && len(l.entries) >= l.cap {
			l.evict(now)
		}
		e = rateEntry{start: now}
	}
	e.n++
	l.entries[ip] = e
	if e.n <= l.count {
		return true, 0
	}
	left := int((l.window - now.Sub(e.start)).Seconds()) + 1
	return false, max(left, 1)
}

func (l *rateLimiter) evict(now time.Time) {
	for k, e := range l.entries {
		if now.Sub(e.start) >= l.window {
			delete(l.entries, k)
		}
	}
	for k := range l.entries {
		if len(l.entries) < l.cap {
			return
		}
		delete(l.entries, k)
	}
}

func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
