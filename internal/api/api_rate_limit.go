package api

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// apiRateLimitBucket is one actor's token-bucket state: capacity refills
// continuously at ratePerMinute, so a legitimate burst (a few requests
// fired back to back) never trips the limiter, only sustained volume
// above the per-minute rate does.
type apiRateLimitBucket struct {
	tokens     float64
	lastRefill time.Time
}

// apiRateLimiter enforces a token-bucket budget per actor key, the same
// in-memory map-plus-mutex shape loginLimiter (ratelimit.go) and
// domainCheckCache (domain_check.go) already use for their own per-key
// throttling. A restart clears every bucket, the same accepted tradeoff
// those two make: this is a single control-plane process (section 4.7),
// not a distributed rate-limit store.
type apiRateLimiter struct {
	mu            sync.Mutex
	buckets       map[string]*apiRateLimitBucket
	ratePerMinute float64 // capacity per window; the name predates window
	window        time.Duration
	maxKeys       int
	now           func() time.Time
	lastSweep     time.Time
}

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
		window: window, maxKeys: defaultLimiterMaxKeys, now: time.Now}
}

// allow reports whether key may proceed right now, and if not, how long
// until its next token is available.
func (l *apiRateLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	if ok, retry := l.peek(key); !ok {
		return false, retry
	}
	l.spend(key)
	return true, 0
}

// peek reports what allow would answer without spending a token.
func (l *apiRateLimiter) peek(key string) (ok bool, retryAfter time.Duration) {
	if l.ratePerMinute <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refilled(key, l.now())
	if b == nil || b.tokens >= 1 {
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.ratePerMinute * float64(l.window))
}

// spend takes one token from key. A race between peek and spend can push
// a bucket below zero, which only lengthens its own wait.
func (l *apiRateLimiter) spend(key string) {
	if l.ratePerMinute <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
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

// allowAll admits a request only when every budget has room, and spends
// from all of them only then, so a rejection by one never drains another.
func allowAll(checks ...limiterCheck) (ok bool, retryAfter time.Duration) {
	for _, c := range checks {
		if ok, retry := c.l.peek(c.key); !ok {
			return false, retry
		}
	}
	for _, c := range checks {
		c.l.spend(c.key)
	}
	return true, 0
}

// apiRateLimitTier buckets an ability into the read or write rate-limit
// budget: AbilityRead is the only read-tier ability, everything else
// (write, write:sensitive, deploy, root) shares the stricter write
// budget, since each of those can mutate real infrastructure, not just
// view it.
func apiRateLimitTier(ability string) string {
	if ability == AbilityRead {
		return "read"
	}
	return "write"
}

// apiRateLimit is the general per-actor request budget requireAbility
// enforces on every route it gates: a compromised or leaked API token
// (or a runaway script) hammering the API surface at unlimited volume is
// throttled here, at the one seam every session- and token-authenticated
// request already funnels through, instead of being bolted onto each
// handler individually. Read-tier and write-tier traffic get independent
// budgets, keyed the same way, so heavy polling of a read endpoint can
// never eat into the budget a write endpoint needs.
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

// writeRateLimited writes the 429 response a rejected apiRateLimit.allow
// call produces: a Retry-After header (RFC 9110 form: an integer count
// of seconds) plus the same {"error": "..."} body shape every other
// error response uses, so a caller parsing responses programmatically
// (internal/apiclient, and eventually the MCP layer) needs no
// status-code-specific parsing to get an actionable message.
func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, http.StatusTooManyRequests, fmt.Sprintf("rate limit exceeded, retry after %ds", seconds))
}
