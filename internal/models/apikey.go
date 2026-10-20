package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	keyPrefix    = "lr-"
	keyRandBytes = 24
	shownPrefix  = 8
)

// NewAPIKey returns a fresh plaintext key, its SHA-256 hex hash, and a
// short non-secret prefix for display. The plaintext is shown once.
func NewAPIKey() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, keyRandBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("models: generate api key: %w", err)
	}
	plaintext = keyPrefix + hex.EncodeToString(buf)
	return plaintext, HashAPIKey(plaintext), plaintext[:shownPrefix], nil
}

// HashAPIKey returns the SHA-256 hex digest stored for a key.
func HashAPIKey(plaintext string) string {
	return tokenhash.Sum(plaintext)
}

// KeyMatches reports whether plaintext hashes to storedHash, in constant
// time.
func KeyMatches(plaintext, storedHash string) bool {
	return tokenhash.Matches(plaintext, storedHash)
}

// NewKeyID returns a random identifier for a virtual key.
func NewKeyID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("models: generate key id: %w", err)
	}
	return "key-" + hex.EncodeToString(buf), nil
}
