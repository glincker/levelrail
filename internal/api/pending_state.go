package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// pendingStateTTL bounds how long a single-use CSRF state value stays
// valid, the same window githubAppRegistrationState uses.
const pendingStateTTL = 15 * time.Minute

// pendingState holds one in-flight browser-redirect OAuth attempt's CSRF
// state value, the same shape githubAppRegistrationState established
// first for the manifest flow's own state parameter. A separate type
// rather than reusing that one: this is a generic single-pending-value
// store any future browser-redirect connect flow can use, not coupled
// to GitHub's manifest-specific naming.
type pendingState struct {
	mu        sync.Mutex
	state     string
	expiresAt time.Time
}

func newPendingState() *pendingState {
	return &pendingState{}
}

func (s *pendingState) begin() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate oauth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	s.state = state
	s.expiresAt = time.Now().Add(pendingStateTTL)
	s.mu.Unlock()

	return state, nil
}

func (s *pendingState) consume(got string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == "" || got == "" {
		return false
	}
	if time.Now().After(s.expiresAt) {
		s.state = ""
		return false
	}
	if got != s.state {
		return false
	}
	s.state = ""
	return true
}
