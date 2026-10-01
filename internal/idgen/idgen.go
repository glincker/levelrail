// Package idgen mints opaque, non-sequential resource identifiers.
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// RandomBytes is the entropy behind every ID: 128 bits keeps IDs
// unguessable and collision-free even across many installs.
const RandomBytes = 16

// New returns prefix followed by RandomBytes of crypto/rand output, hex encoded.
func New(prefix string) (string, error) {
	b := make([]byte, RandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("idgen: generate %sid: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(b), nil
}
