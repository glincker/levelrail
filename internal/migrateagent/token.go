// Package migrateagent is the control plane side of the read-only migration
// agent: single-use tokens, the mTLS session service, discovery storage and the
// volume comparison that advances the cutover checklist.
package migrateagent

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	sessionIDRe = regexp.MustCompile(`^ms_[0-9a-f]{16}$`)
	secretRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

const (
	tokenPrefix = "mig_"
	// CertCommonNamePrefix marks a migration client certificate. A node
	// certificate never carries it, so neither kind opens the other's stream.
	CertCommonNamePrefix = "migration-source:"
)

// NewToken returns a session id, the plaintext token to show once, and the hash to store.
func NewToken() (id, token, hash string, err error) {
	rawID := make([]byte, 8)
	secret := make([]byte, 32)
	if _, err = rand.Read(rawID); err != nil {
		return "", "", "", fmt.Errorf("migrateagent: random id: %w", err)
	}
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", fmt.Errorf("migrateagent: random secret: %w", err)
	}
	id = "ms_" + hex.EncodeToString(rawID)
	sec := base64.RawURLEncoding.EncodeToString(secret)
	return id, tokenPrefix + id + "." + sec, HashSecret(sec), nil
}

// ParseToken splits a token into its session id and secret.
func ParseToken(token string) (id, secret string, ok bool) {
	rest, found := strings.CutPrefix(token, tokenPrefix)
	if !found {
		return "", "", false
	}
	id, secret, found = strings.Cut(rest, ".")
	if !found || !sessionIDRe.MatchString(id) || !secretRe.MatchString(secret) {
		return "", "", false
	}
	return id, secret, true
}

// HashSecret is the stored form of a token secret.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// SecretMatches compares a presented secret with a stored hash in constant time.
func SecretMatches(storedHash, secret string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(HashSecret(secret))) == 1
}
