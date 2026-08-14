package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginGraceFailures, loginBaseBackoff, and loginMaxBackoff shape the
// backoff curve: a handful of free tries (typos happen), then real
// exponential growth, capped, not CapRover's flat 30-second global
// back-off after 5 failures with no per-IP separation
// (docs-local/research/competitor-onboarding-auth-ux.md finding 8).
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
)

// loginAttemptState is one (IP, username) pair's failure history.
type loginAttemptState struct {
	failures    int
	lockedUntil time.Time
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

// recordSuccess clears key's failure history entirely: a successful
// login is a clean slate, not a partial credit toward the next lockout.
func (l *loginLimiter) recordSuccess(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// loginLimiterKey combines the client IP and the attempted username, so
// a single misbehaving IP can't lock a legitimate account out from a
// different IP by deliberately failing logins for it, and an attacker
// spreading attempts across many IPs still only gets loginGraceFailures
// free tries per IP, not an unlimited pool shared across all of them.
func loginLimiterKey(r *http.Request, username string) string {
	return clientIP(r) + "|" + username
}

// clientIP extracts the request's IP, stripping the port from
// RemoteAddr. Deliberately does not consult X-Forwarded-For: trusting
// that header safely requires knowing how many proxy hops are in front
// and stripping anything past the trusted one, since it's otherwise
// trivially spoofable to defeat this exact limiter. Caddy sits in
// front of every real deployment's app ingress, but the
// control plane's own admin listener isn't behind it yet; wiring a
// specific trusted-hop count is a follow-up for when that topology
// exists, not a default to guess at now.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
