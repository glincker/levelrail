// Package ids generates random identifiers from crypto/rand.
package ids

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("ids: read random bytes: %w", err)
	}
	return b, nil
}

// Hex returns n random bytes as 2n lowercase hex characters.
func Hex(n int) (string, error) {
	b, err := randBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MustHex is Hex for callers with no error path; it panics if the system
// random source fails, which crypto/rand treats as unrecoverable anyway.
func MustHex(n int) string {
	s, err := Hex(n)
	if err != nil {
		panic(err)
	}
	return s
}

// HexLen returns exactly length lowercase hex characters.
func HexLen(length int) (string, error) {
	s, err := Hex((length + 1) / 2)
	if err != nil {
		return "", err
	}
	return s[:length], nil
}

// URL returns n random bytes as unpadded URL-safe base64.
func URL(n int) (string, error) {
	b, err := randBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// LowerText returns length lowercase base32 characters (at most 26).
func LowerText(length int) string {
	return strings.ToLower(rand.Text()[:length])
}
