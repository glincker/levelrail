package api

import (
	"fmt"
	"sync"
	"time"
)

const (
	mfaPendingTTL = 5 * time.Minute
)

// mfaPending identifies who is mid-login after a correct password but
// before a correct second factor: a session cookie is not issued until
// handleVerifyTwoFactor consumes one of these successfully.
type mfaPending struct {
	userID    string
	expiresAt time.Time
}

// mfaPendingStore is a short-TTL, in-memory sibling of sessionStore,
// same shape, much narrower purpose: it identifies a mid-login user,
// nothing else, and expires in minutes rather than hours.
type mfaPendingStore struct {
	mu      sync.Mutex
	pending map[string]mfaPending
}

func newMFAPendingStore() *mfaPendingStore {
	return &mfaPendingStore{pending: make(map[string]mfaPending)}
}

func (s *mfaPendingStore) create(userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate mfa pending token: %w", err)
	}
	s.mu.Lock()
	s.pending[token] = mfaPending{userID: userID, expiresAt: time.Now().Add(mfaPendingTTL)}
	s.mu.Unlock()
	return token, nil
}

// lookup does not delete on read: a mistyped code must not burn the
// pending token, the caller gets to keep retrying (subject to
// rt.mfaVerify's rate limit) until it expires or a correct one lands.
func (s *mfaPendingStore) lookup(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[token]
	if !ok {
		return "", false
	}
	if time.Now().After(p.expiresAt) {
		delete(s.pending, token)
		return "", false
	}
	return p.userID, true
}

func (s *mfaPendingStore) revoke(token string) {
	s.mu.Lock()
	delete(s.pending, token)
	s.mu.Unlock()
}

type twoFactorStatusResponse struct {
	Enabled                bool `json:"enabled"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

type twoFactorSetupResponse struct {
	Secret          string `json:"secret"`
	ProvisioningURI string `json:"provisioning_uri"`
}

type twoFactorCodeRequest struct {
	Code string `json:"code"`
}

type twoFactorRecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

type twoFactorDisableRequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

type twoFactorVerifyRequest struct {
	MFAToken     string `json:"mfa_token"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}
