package abuse

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

// Clock returns monotonic nanoseconds. Tests inject a fake one.
type Clock func() int64

var epoch = time.Now()

// MonotonicClock is the production clock.
func MonotonicClock() int64 { return int64(time.Since(epoch)) }

type bucket struct {
	tokens float64
	last   int64
}

type shard struct {
	mu        sync.Mutex
	m         map[netip.Addr]*bucket
	lastSweep int64
	// Keeps neighbouring shards off one cache line.
	_ [40]byte
}

// Limiter is a sharded per-client token bucket. Each shard has its own lock
// and its own bounded map, so there is no global lock and memory is capped at
// the configured client count. A shard drops buckets idle for longer than
// idle on a timed sweep, and evicts an arbitrary bucket when it is full.
type Limiter struct {
	shards   []shard
	mask     uint64
	perShard int
	idle     int64
	sweep    int64
	clock    Clock
	evicted  atomic.Uint64
}

// NewLimiter tracks at most maxClients addresses across shards shards (rounded
// up to a power of two). A nil clock uses MonotonicClock.
func NewLimiter(maxClients, shards int, idle time.Duration, clock Clock) *Limiter {
	n := 1
	for n < max(shards, 1) {
		n <<= 1
	}
	if clock == nil {
		clock = MonotonicClock
	}
	l := &Limiter{
		shards: make([]shard, n), mask: uint64(n - 1), clock: clock, //nolint:gosec // n is a positive power of two
		perShard: max(maxClients/n, 1), idle: int64(idle), sweep: int64(idle / 4),
	}
	for i := range l.shards {
		l.shards[i].m = make(map[netip.Addr]*bucket)
	}
	return l
}

func hashAddr(a netip.Addr) uint64 {
	b := a.As16()
	lo := binary.LittleEndian.Uint64(b[:8])
	hi := binary.LittleEndian.Uint64(b[8:])
	h := lo ^ (hi * 0x9e3779b97f4a7c15)
	h ^= h >> 32
	h *= 0xd6e8feb86659fd93
	h ^= h >> 32
	h *= 0xd6e8feb86659fd93
	return h ^ (h >> 32)
}

// Allow takes one token for addr from a bucket refilling at rps up to burst.
// When it refuses, retryAfter is how long until a token exists.
func (l *Limiter) Allow(addr netip.Addr, rps float64, burst int) (ok bool, retryAfter time.Duration) {
	now := l.clock()
	sh := &l.shards[hashAddr(addr)&l.mask]
	sh.mu.Lock()
	if now-sh.lastSweep > l.sweep {
		l.sweepLocked(sh, now)
	}
	b := sh.m[addr]
	if b == nil {
		if len(sh.m) >= l.perShard {
			l.evictLocked(sh)
		}
		b = &bucket{tokens: float64(burst), last: now}
		sh.m[addr] = b
	} else {
		b.tokens += float64(now-b.last) * rps / float64(time.Second)
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		sh.mu.Unlock()
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / rps * float64(time.Second))
	sh.mu.Unlock()
	return false, wait
}

func (l *Limiter) sweepLocked(sh *shard, now int64) {
	sh.lastSweep = now
	for k, b := range sh.m {
		if now-b.last > l.idle {
			delete(sh.m, k)
		}
	}
}

func (l *Limiter) evictLocked(sh *shard) {
	for k := range sh.m {
		delete(sh.m, k)
		l.evicted.Add(1)
		return
	}
}

// Len is the number of clients currently tracked.
func (l *Limiter) Len() int {
	n := 0
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		n += len(sh.m)
		sh.mu.Unlock()
	}
	return n
}

// Evicted is how many buckets were dropped because a shard was full.
func (l *Limiter) Evicted() uint64 { return l.evicted.Load() }
