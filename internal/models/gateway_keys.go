package models

import (
	"crypto/subtle"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// authenticate returns the key whose hash matches token. Every key is
// compared so the time taken does not reveal which one, or how many, matched.
// A revoked or expired key is treated as unknown.
func authenticate(keys []store.ModelKey, token string, now time.Time) (store.ModelKey, bool) {
	h := []byte(HashAPIKey(token))
	var found *store.ModelKey
	for i := range keys {
		if subtle.ConstantTimeCompare(h, []byte(keys[i].KeyHash)) == 1 {
			found = &keys[i]
		}
	}
	if found == nil || !keyUsable(*found, now) {
		return store.ModelKey{}, false
	}
	return *found, true
}

func keyUsable(k store.ModelKey, now time.Time) bool {
	if k.RevokedAt != nil && !now.Before(*k.RevokedAt) {
		return false
	}
	return k.ExpiresAt == nil || now.Before(*k.ExpiresAt)
}

// pathAllowed reports whether k may call path. An entry ending in "/"
// allows every path below it.
func pathAllowed(k store.ModelKey, path string) bool {
	if len(k.AllowPaths) == 0 {
		return true
	}
	for _, p := range k.AllowPaths {
		if p == path || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) && len(path) > len(p)) {
			return true
		}
	}
	return false
}

// modelAllowed reports whether k may use the model named in a request body.
// A request that names no model passes only when it has no body to name
// one in (a listing call).
func modelAllowed(k store.ModelKey, requested string, hasBody bool) bool {
	if len(k.AllowModels) == 0 {
		return true
	}
	if requested == "" {
		return !hasBody
	}
	return slices.Contains(k.AllowModels, requested)
}

// ValidateAllowPaths reports the first entry that is not a path the
// engine's gateway allowlist forwards.
func ValidateAllowPaths(engine string, paths []string) (string, bool) {
	served := map[string]bool{}
	for _, r := range routesFor(engine) {
		served[r.path] = true
	}
	for _, p := range paths {
		if !served[p] {
			return p, false
		}
	}
	return "", true
}

// keyLimiter enforces per-key request rate and parallelism in memory and
// tracks a per-minute token count fed by response metering.
type keyLimiter struct {
	mu     sync.Mutex
	states map[string]*keyState
}

type keyState struct {
	tokens      float64
	refilled    time.Time
	parallel    int
	windowStart time.Time
	windowUsed  int64
}

func (l *keyLimiter) state(id string, now time.Time, burst int) *keyState {
	if l.states == nil {
		l.states = map[string]*keyState{}
	}
	s := l.states[id]
	if s == nil {
		s = &keyState{tokens: float64(burst), refilled: now, windowStart: now}
		l.states[id] = s
	}
	return s
}

// acquire admits one request for k or reports how long to wait. The
// returned release must be called when the request ends. tpm is a soft
// limit: it blocks once the current minute's metered tokens reach it.
func (l *keyLimiter) acquire(k store.ModelKey, now time.Time) (release func(), retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state(k.ID, now, k.RPM)
	if k.TPM > 0 {
		if now.Sub(s.windowStart) >= time.Minute {
			s.windowStart, s.windowUsed = now, 0
		}
		if s.windowUsed >= int64(k.TPM) {
			return nil, s.windowStart.Add(time.Minute).Sub(now), false
		}
	}
	if k.MaxParallel > 0 && s.parallel >= k.MaxParallel {
		return nil, time.Second, false
	}
	if k.RPM > 0 {
		rate := float64(k.RPM) / 60
		s.tokens = min(float64(k.RPM), s.tokens+now.Sub(s.refilled).Seconds()*rate)
		s.refilled = now
		if s.tokens < 1 {
			return nil, time.Duration((1 - s.tokens) / rate * float64(time.Second)), false
		}
		s.tokens--
	}
	s.parallel++
	id := k.ID
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if st := l.states[id]; st != nil && st.parallel > 0 {
			st.parallel--
		}
	}, 0, true
}

// addTokens counts metered tokens against the key's current minute.
func (l *keyLimiter) addTokens(id string, n int64, now time.Time) {
	if n <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.states[id]
	if s == nil {
		return
	}
	if now.Sub(s.windowStart) >= time.Minute {
		s.windowStart, s.windowUsed = now, 0
	}
	s.windowUsed += n
}

// inFlight returns the running request count of every key with any.
func (l *keyLimiter) inFlight() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]int{}
	for id, s := range l.states {
		if s.parallel > 0 {
			out[id] = s.parallel
		}
	}
	return out
}

// retain drops state of keys that no longer exist and are idle.
func (l *keyLimiter) retain(live map[string]bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, s := range l.states {
		if !live[id] && s.parallel == 0 {
			delete(l.states, id)
		}
	}
}
