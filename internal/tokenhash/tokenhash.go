// Package tokenhash is the one SHA-256 hex digest used for stored secrets.
package tokenhash

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// Sum returns the hex-encoded SHA-256 digest of plaintext.
func Sum(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Matches reports whether plaintext hashes to storedHash, in constant time.
func Matches(plaintext, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(Sum(plaintext)), []byte(storedHash)) == 1
}
