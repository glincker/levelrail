package trafficpolicy

import (
	"container/list"
	"net/netip"
	"sync"
	"time"
)

type ipState struct {
	addr     netip.Addr
	tokens   float64
	last     time.Time
	inflight int
}

// IPLimiter is a per-client-IP token bucket and concurrency counter. State
// lives only in memory and is capped: the least recently seen address is
// evicted first, so a flood of distinct addresses cannot grow it.
type IPLimiter struct {
	mu       sync.Mutex
	capacity int
	now      func() time.Time
	order    *list.List
	byAddr   map[netip.Addr]*list.Element
}

// NewIPLimiter tracks at most capacity addresses. A nil now uses time.Now.
func NewIPLimiter(capacity int, now func() time.Time) *IPLimiter {
	if capacity < 1 {
		capacity = 1
	}
	if now == nil {
		now = time.Now
	}
	return &IPLimiter{capacity: capacity, now: now, order: list.New(), byAddr: make(map[netip.Addr]*list.Element)}
}

// Len is the number of addresses currently tracked.
func (l *IPLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

func (l *IPLimiter) touch(addr netip.Addr, burst int, now time.Time) *ipState {
	if el, ok := l.byAddr[addr]; ok {
		l.order.MoveToFront(el)
		return el.Value.(*ipState)
	}
	for l.order.Len() >= l.capacity {
		oldest := l.order.Back()
		delete(l.byAddr, oldest.Value.(*ipState).addr)
		l.order.Remove(oldest)
	}
	st := &ipState{addr: addr, tokens: float64(burst), last: now}
	l.byAddr[addr] = l.order.PushFront(st)
	return st
}

// Acquire admits one request from addr, or reports why not. On success the
// caller must call release when the request finishes. ratePerMinute 0 skips
// the rate check and maxConcurrent 0 skips the concurrency check.
func (l *IPLimiter) Acquire(addr netip.Addr, ratePerMinute, burst, maxConcurrent int) (release func(), reason ShieldReason, ok bool) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.touch(addr, burst, now)
	if maxConcurrent > 0 && st.inflight >= maxConcurrent {
		return nil, ShieldConcurrency, false
	}
	if ratePerMinute > 0 {
		st.tokens += now.Sub(st.last).Seconds() * float64(ratePerMinute) / 60
		if st.tokens > float64(burst) {
			st.tokens = float64(burst)
		}
		st.last = now
		if st.tokens < 1 {
			return nil, ShieldRateLimit, false
		}
		st.tokens--
	}
	st.inflight++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			st.inflight--
			l.mu.Unlock()
		})
	}, 0, true
}
