package api

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// webhookBodySeen remembers recent signed bodies so a captured request
// cannot be replayed with a changed or missing delivery header, which the
// signature does not cover. The zero value is ready to use.
type webhookBodySeen struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func webhookBodyKey(provider, app string, body []byte) string {
	sum := sha256.Sum256(body)
	return provider + "|" + app + "|" + hex.EncodeToString(sum[:])
}

// claim records the body and reports true, or reports false when the same
// body was claimed within window.
func (s *webhookBodySeen) claim(key string, now time.Time, window time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string]time.Time{}
	}
	for k, at := range s.seen {
		if now.Sub(at) >= window {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[key]; ok {
		return false
	}
	s.seen[key] = now
	return true
}

// release forgets a body so a failed delivery can be retried by the provider.
func (s *webhookBodySeen) release(key string) {
	s.mu.Lock()
	delete(s.seen, key)
	s.mu.Unlock()
}
