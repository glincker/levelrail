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
	ratePerMinute float64
}

// newAPIRateLimiter builds an apiRateLimiter. ratePerMinute <= 0 means
// "no limit", so callers can construct one unconditionally and let allow
// always report true rather than branching on it being absent.
func newAPIRateLimiter(ratePerMinute int) *apiRateLimiter {
	return &apiRateLimiter{buckets: make(map[string]*apiRateLimitBucket), ratePerMinute: float64(ratePerMinute)}
}

// allow reports whether key may proceed right now, and if not, how long
// until its next token is available.
func (l *apiRateLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	if l.ratePerMinute <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[key]
	if !exists {
		l.buckets[key] = &apiRateLimitBucket{tokens: l.ratePerMinute - 1, lastRefill: now}
		return true, 0
	}

	b.tokens += now.Sub(b.lastRefill).Minutes() * l.ratePerMinute
	if b.tokens > l.ratePerMinute {
		b.tokens = l.ratePerMinute
	}
	b.lastRefill = now

	if b.tokens < 1 {
		wait := time.Duration((1 - b.tokens) / l.ratePerMinute * float64(time.Minute))
		return false, wait
	}
	b.tokens--
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
