package api

import (
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

// loginGraceFailures, loginBaseBackoff, and loginMaxBackoff shape the
// backoff curve: a handful of free tries (typos happen), then real
// exponential growth, capped, not CapRover's flat 30-second global
// back-off after 5 failures with no per-IP separation.
const (
	loginGraceFailures = 3
	loginBaseBackoff   = 1 * time.Second
	loginMaxBackoff    = 15 * time.Minute
	// maxBackoffShift bounds the exponent loginLimiter.recordFailure
	// ever left-shifts loginBaseBackoff by. time.Duration is an int64
	// count of nanoseconds; an unbounded shift (a sustained attack
	// racking up hundreds of failures) would overflow it long before
	// the post-shift "cap at loginMaxBackoff" check ever runs. 20 is
	// already far past the point loginMaxBackoff kicks in
	// (1s << 20 ≈ 12 days), so the clamp below is what actually decides
	// the real ceiling; this only exists to keep the arithmetic itself
	// safe on the way there.
	maxBackoffShift = 20
	// loginLimiterPruneAt is the map size that triggers a sweep of idle entries.
	loginLimiterPruneAt = 4096
	// loginLimiterIdleAfter is how long an unlocked key must be quiet before a sweep drops it.
	loginLimiterIdleAfter = time.Hour
	// devicePollPerMinute caps POST /auth/device/token per client IP (a CLI polls about every 5s).
	devicePollPerMinute = 120
)

// loginAttemptState is one (IP, username) pair's failure history.
type loginAttemptState struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

// loginLimiter enforces exponential backoff per (client IP, attempted
// username) pair. In-memory, the same accepted tradeoff sessionStore
// already makes for a single-node control plane: a restart clears every
// counter, which only means an attacker (or a confused operator) gets
// their grace failures back, not a security regression worth a
// persistent store for.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttemptState
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttemptState)}
}

// allow reports whether a login attempt for key may proceed right now,
// and if not, how much longer the caller must wait.
func (l *loginLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, exists := l.attempts[key]
	if !exists {
		return true, 0
	}
	if remaining := time.Until(st.lockedUntil); remaining > 0 {
		return false, remaining
	}
	return true, 0
}

// recordFailure records a failed attempt for key, computing the next
// backoff window once failures exceed loginGraceFailures.
func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, exists := l.attempts[key]
	if !exists {
		st = &loginAttemptState{}
		l.attempts[key] = st
	}
	st.failures++
	st.lastFailure = time.Now()
	if len(l.attempts) >= loginLimiterPruneAt {
		l.pruneLocked(st.lastFailure)
	}
	if st.failures <= loginGraceFailures {
		return
	}

	shift := st.failures - loginGraceFailures - 1
	if shift > maxBackoffShift {
		shift = maxBackoffShift
	}
	backoff := loginBaseBackoff << shift
	if backoff > loginMaxBackoff {
		backoff = loginMaxBackoff
	}
	st.lockedUntil = time.Now().Add(backoff)
}

// pruneLocked drops keys that are no longer locked and have been quiet for loginLimiterIdleAfter,
// so a flood of one-off keys cannot grow the map without bound. Callers hold l.mu.
func (l *loginLimiter) pruneLocked(now time.Time) {
	for k, st := range l.attempts {
		if now.After(st.lockedUntil) && now.Sub(st.lastFailure) > loginLimiterIdleAfter {
			delete(l.attempts, k)
		}
	}
}

// recordSuccess clears key's failure history entirely: a successful
// login is a clean slate, not a partial credit toward the next lockout.
func (l *loginLimiter) recordSuccess(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// allowTokenRedeem applies the per-IP token-redeem budget, writing the
// 429 itself and returning false when the caller is over it.
func (rt *Router) allowTokenRedeem(w http.ResponseWriter, r *http.Request, route string) bool {
	if rt.tokenRedeemRateLimit == nil {
		return true
	}
	allowed, retryAfter := rt.tokenRedeemRateLimit.allow(route + "|" + clientIP(r))
	if !allowed {
		writeRateLimited(w, retryAfter)
	}
	return allowed
}

// loginLimiterKey combines the client IP and the attempted username, so
// a single misbehaving IP can't lock a legitimate account out from a
// different IP by deliberately failing logins for it, and an attacker
// spreading attempts across many IPs still only gets loginGraceFailures
// free tries per IP, not an unlimited pool shared across all of them.
func loginLimiterKey(r *http.Request, username string) string {
	return clientIP(r) + "|" + username
}

// clientIP is the request's real client address. X-Forwarded-For is only
// read when the TCP peer is loopback or a configured trusted proxy, and the
// right-most untrusted hop wins so a client-supplied prefix cannot spoof it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	trusted := trustedProxyPrefixes()
	if !isTrustedPeer(host, trusted) {
		return host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return host
		}
		if !isTrustedPeer(addr.String(), trusted) {
			return addr.String()
		}
	}
	return host
}

func isTrustedPeer(host string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() && loopbackTrusted() {
		return true
	}
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func trustedProxyPrefixes() []netip.Prefix {
	var out []netip.Prefix
	for _, e := range strings.Split(os.Getenv(envTrustedProxies), ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			out = append(out, p)
		} else if a, err := netip.ParseAddr(e); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}
