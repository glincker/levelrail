package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// pendingStateTTL bounds how long a single-use CSRF state value stays
// valid: generous enough to cover an operator's own confirmation click
// on the provider's authorization page, short enough that an abandoned
// connect attempt doesn't leave a usable CSRF token lying around
// indefinitely.
const pendingStateTTL = 15 * time.Minute

// pendingState holds in-flight browser-redirect connect attempts' CSRF
// state values, each with a caller-supplied value carried from begin to
// consume: a GitHub registration's instance URL, or a git provider
// connection id. In-memory, since every connect flow is a short round trip
// through the browser.
type pendingState struct {
	mu      sync.Mutex
	entries map[string]pendingEntry
}

type pendingEntry struct {
	value     string
	expiresAt time.Time
}

func newPendingState() *pendingState {
	return &pendingState{entries: map[string]pendingEntry{}}
}

// begin registers a fresh single-use state. Several can be pending at once,
// so two accounts can be authorized in parallel without clobbering each other.
func (s *pendingState) begin(value string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate oauth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.entries {
		if now.After(e.expiresAt) {
			delete(s.entries, k)
		}
	}
	s.entries[state] = pendingEntry{value: value, expiresAt: now.Add(pendingStateTTL)}
	return state, nil
}

// consume reports whether got is a pending, unexpired state and removes it,
// returning the value passed to begin. A wrong value never disturbs the
// states still in flight.
func (s *pendingState) consume(got string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if got == "" {
		return "", false
	}
	e, ok := s.entries[got]
	if !ok {
		return "", false
	}
	delete(s.entries, got)
	if time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}
