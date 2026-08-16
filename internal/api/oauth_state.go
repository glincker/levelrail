package api

import (
	"sync"
	"time"
)

// OAuth sign-in purposes an oauthState can carry: signin is the
// anonymous flow (handleOAuthStart); link is the authenticated
// "attach this provider to my account" flow (handleOAuthLinkStart).
const (
	oauthPurposeSignin = "signin"
	oauthPurposeLink   = "link"
)

// oauthStateTTL bounds how long an operator has to complete the
// provider's consent screen. Fixed: a security floor, not an env var.
const oauthStateTTL = 10 * time.Minute

// oauthState is one in-flight OAuth authorization attempt, keyed by the
// random state token sent to the provider and returned on its callback.
// In-memory like sessionStore: a restart just means clicking sign-in again.
type oauthState struct {
	provider   string
	purpose    string
	linkUserID string
	expiresAt  time.Time
}

type oauthStateStore struct {
	mu     sync.Mutex
	states map[string]oauthState
}

func newOAuthStateStore() *oauthStateStore {
	return &oauthStateStore{states: make(map[string]oauthState)}
}

func (s *oauthStateStore) create(provider, purpose, linkUserID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.states[token] = oauthState{
		provider:   provider,
		purpose:    purpose,
		linkUserID: linkUserID,
		expiresAt:  time.Now().Add(oauthStateTTL),
	}
	s.mu.Unlock()
	return token, nil
}

// consume deletes token in the same locked section as the lookup:
// single-use by construction, so a replayed callback finds nothing.
func (s *oauthStateStore) consume(token string) (oauthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[token]
	if !ok {
		return oauthState{}, false
	}
	delete(s.states, token)
	if time.Now().After(st.expiresAt) {
		return oauthState{}, false
	}
	return st, true
}
