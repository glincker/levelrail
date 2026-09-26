package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
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
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// KeyMatches reports whether plaintext hashes to storedHash, in constant
// time.
func KeyMatches(plaintext, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashAPIKey(plaintext)), []byte(storedHash)) == 1
}

// NewKeyID returns a random identifier for a virtual key.
func NewKeyID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("models: generate key id: %w", err)
	}
	return "key-" + hex.EncodeToString(buf), nil
}
