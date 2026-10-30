package advisoryfeed

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Signature states shown wherever a feed is shown. Unsigned means no public
// key is configured, so the content is not authenticated.
const (
	SigUnsigned = "unsigned"
	SigVerified = "verified"
	SigInvalid  = "invalid"
)

// ParsePublicKey decodes a base64 ed25519 public key from configuration.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("advisory feed public key is not base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("advisory feed public key must be a 32 byte ed25519 key")
	}
	return ed25519.PublicKey(raw), nil
}

// Verify checks a base64 detached ed25519 signature over the exact feed bytes.
func Verify(key ed25519.PublicKey, data, sigB64 []byte) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("advisory feed signature is not base64: %w", err)
	}
	if !ed25519.Verify(key, data, sig) {
		return errors.New("advisory feed signature does not match the configured key")
	}
	return nil
}
