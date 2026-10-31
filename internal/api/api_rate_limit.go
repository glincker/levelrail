package api

import (
	"cmp"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// apiRateLimitBucket is one actor's token bucket, refilled continuously so short bursts pass.
type apiRateLimitBucket struct {
	tokens     float64
	lastRefill time.Time
}

// apiRateLimiter is an in-memory per-actor token bucket. A restart clears all buckets, which is accepted for one control-plane process.
type apiRateLimiter struct {
	mu            sync.Mutex
	buckets       map[string]*apiRateLimitBucket
	ratePerMinute float64 // capacity per window; the name predates window
	window        time.Duration
	maxKeys       int
	now           func() time.Time
	lastSweep     time.Time
	order         uint64 // fixed lock order for allowAll
}

var limiterOrder atomic.Uint64

// defaultLimiterMaxKeys caps one limiter's memory when a flood of distinct
// keys arrives faster than idle buckets age out.
const defaultLimiterMaxKeys = 50_000

// newAPIRateLimiter builds an apiRateLimiter. ratePerMinute <= 0 means
// "no limit", so callers can construct one unconditionally and let allow
// always report true rather than branching on it being absent.
func newAPIRateLimiter(ratePerMinute int) *apiRateLimiter {
	return newWindowRateLimiter(ratePerMinute, time.Minute)
}

// newWindowRateLimiter allows capacity requests per window per key.
func newWindowRateLimiter(capacity int, window time.Duration) *apiRateLimiter {
	return &apiRateLimiter{buckets: make(map[string]*apiRateLimitBucket), ratePerMinute: float64(capacity),
		window: window, maxKeys: defaultLimiterMaxKeys, now: time.Now, order: limiterOrder.Add(1)}
}

// allow reports whether key may proceed right now, and if not, how long
// until its next token is available.
func (l *apiRateLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	if l.ratePerMinute <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if ok, retry := l.hasLocked(key, 1); !ok {
		return false, retry
	}
	l.spendLocked(key)
	return true, 0
}

// peek reports whether key has a token left without spending it.
func (l *apiRateLimiter) peek(key string) bool {
	if l.ratePerMinute <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ok, _ := l.hasLocked(key, 1)
	return ok
}

// spend takes one token from key unconditionally, for callers that count only failures.
func (l *apiRateLimiter) spend(key string) {
	if l.ratePerMinute <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spendLocked(key)
}

// hasLocked reports whether key holds n tokens, and if not how long until it
// will. Callers hold l.mu.
func (l *apiRateLimiter) hasLocked(key string, n float64) (ok bool, retryAfter time.Duration) {
	tokens := l.ratePerMinute
	if b := l.refilled(key, l.now()); b != nil {
		tokens = b.tokens
	}
	if tokens >= n {
		return true, 0
	}
	return false, time.Duration((n - tokens) / l.ratePerMinute * float64(l.window))
}

// spendLocked takes one token from key. Callers hold l.mu.
func (l *apiRateLimiter) spendLocked(key string) {
	now := l.now()
	b := l.refilled(key, now)
	if b == nil {
		l.sweep(now)
		b = &apiRateLimitBucket{tokens: l.ratePerMinute, lastRefill: now}
		l.buckets[key] = b
	}
	b.tokens--
}

// refilled returns key's bucket topped up to now, or nil for a key with
// no bucket (which reads as full). Callers hold l.mu.
func (l *apiRateLimiter) refilled(key string, now time.Time) *apiRateLimitBucket {
	b, ok := l.buckets[key]
	if !ok {
		return nil
	}
	b.tokens += float64(now.Sub(b.lastRefill)) / float64(l.window) * l.ratePerMinute
	if b.tokens > l.ratePerMinute {
		b.tokens = l.ratePerMinute
	}
	b.lastRefill = now
	return b
}

// sweep drops buckets idle for a full window (they are full again, the same
// as absent) and, past maxKeys, arbitrary ones. Callers hold l.mu.
func (l *apiRateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) >= l.window || len(l.buckets) >= l.maxKeys {
		for k, b := range l.buckets {
			if now.Sub(b.lastRefill) >= l.window {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	for k := range l.buckets {
		if len(l.buckets) < l.maxKeys {
			break
		}
		delete(l.buckets, k)
	}
}

// keyCount reports how many buckets the limiter holds.
func (l *apiRateLimiter) keyCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// limiterCheck is one budget a request must fit.
type limiterCheck struct {
	l   *apiRateLimiter
	key string
}

// allowAll spends from every budget only if all have room, so one rejection never drains another. Locks are taken in creation order.
func allowAll(checks ...limiterCheck) (ok bool, retryAfter time.Duration) {
	need := make(map[limiterCheck]float64, len(checks))
	var limiters []*apiRateLimiter
	for _, c := range checks {
		if c.l.ratePerMinute <= 0 {
			continue
		}
		if !slices.Contains(limiters, c.l) {
			limiters = append(limiters, c.l)
		}
		need[c]++
	}
	slices.SortFunc(limiters, func(a, b *apiRateLimiter) int { return cmp.Compare(a.order, b.order) })
	for _, l := range limiters {
		l.mu.Lock()
		defer l.mu.Unlock()
	}
	for _, c := range checks {
		if n, ok := need[c]; ok {
			if ok, retry := c.l.hasLocked(c.key, n); !ok {
				return false, retry
			}
		}
	}
	for _, c := range checks {
		if c.l.ratePerMinute > 0 {
			c.l.spendLocked(c.key)
		}
	}
	return true, 0
}

// apiRateLimitTier maps an ability to the read budget (AbilityRead) or the stricter write budget (everything else).
func apiRateLimitTier(ability string) string {
	if ability == AbilityRead {
		return "read"
	}
	return "write"
}

// apiRateLimit is the per-actor budget requireAbility enforces on every gated route. Read and write tiers are budgeted independently.
type apiRateLimit struct {
	reads  *apiRateLimiter
	writes *apiRateLimiter
}

// newAPIRateLimit builds an apiRateLimit. Either argument <= 0 disables
// that tier's limit.
func newAPIRateLimit(readPerMinute, writePerMinute int) *apiRateLimit {
	return &apiRateLimit{reads: newAPIRateLimiter(readPerMinute), writes: newAPIRateLimiter(writePerMinute)}
}

func (l *apiRateLimit) allow(ability, key string) (ok bool, retryAfter time.Duration) {
	if apiRateLimitTier(ability) == "read" {
		return l.reads.allow(key)
	}
	return l.writes.allow(key)
}

// admit is allow plus X-RateLimit-* headers on success, read from the same bucket so they never disagree with enforcement.
func (l *apiRateLimit) admit(w http.ResponseWriter, ability, key string) (ok bool, retryAfter time.Duration) {
	lim := l.writes
	if apiRateLimitTier(ability) == "read" {
		lim = l.reads
	}
	ok, retryAfter = lim.allow(key)
	if ok {
		lim.setRateLimitHeaders(w, key)
	}
	return ok, retryAfter
}

// setRateLimitHeaders reports the budget, tokens left and seconds until the bucket is full again (Reset is a delta, not an epoch).
func (l *apiRateLimiter) setRateLimitHeaders(w http.ResponseWriter, key string) {
	if l.ratePerMinute <= 0 {
		return
	}
	l.mu.Lock()
	tokens := l.ratePerMinute
	if b := l.refilled(key, l.now()); b != nil {
		tokens = b.tokens
	}
	l.mu.Unlock()
	remaining := max(int(tokens), 0)
	reset := int(math.Ceil((l.ratePerMinute - tokens) / l.ratePerMinute * l.window.Seconds()))
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(int(l.ratePerMinute)))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	h.Set("X-RateLimit-Reset", strconv.Itoa(max(reset, 0)))
}

// writeRateLimited writes a 429 with a Retry-After header and the standard JSON error body.
func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, http.StatusTooManyRequests, fmt.Sprintf("rate limit exceeded, retry after %ds", seconds))
}

// writeBudget charges a credential-touching route that is gated at AbilityRead
// (so the audit seam and read tier pass it) to the stricter write budget.
func (rt *Router) writeBudget(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt.apiRateLimit != nil {
			key := "cred:ip:" + clientIP(r)
			if uid, ok := rt.currentSessionUserID(r); ok {
				key = "cred:user:" + uid
			}
			if allowed, retryAfter := rt.apiRateLimit.writes.allow(key); !allowed {
				writeRateLimited(w, retryAfter)
				return
			}
		}
		next(w, r)
	}
}
