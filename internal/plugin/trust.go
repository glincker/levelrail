package plugin

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Verification failures. Callers match with errors.Is.
var (
	ErrUntrusted      = errors.New("plugin: signing key is not in the trust list")
	ErrBadSignature   = errors.New("plugin: signature does not verify")
	ErrDigestMismatch = errors.New("plugin: image digest does not match the pinned digest")
)

const signedMessagePrefix = "plugin-package-v1"

// TrustedKey is one operator-approved publisher key.
type TrustedKey struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	AddedAt   string `json:"added_at"`
	AddedBy   string `json:"added_by"`
}

// Fingerprint is the stable identifier of an ed25519 public key.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "ed25519:" + hex.EncodeToString(sum[:])
}

// ParsePublicKey decodes a standard base64 raw 32 byte ed25519 public key.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, invalid("public key is not base64: %v", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, invalid("public key must be %d raw bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// SignedMessage is the exact byte string a publisher signs: a domain tag, the
// sha256 of the manifest bytes as shipped, and the pinned image digest.
func SignedMessage(manifestRaw []byte, digest string) []byte {
	sum := sha256.Sum256(manifestRaw)
	return []byte(signedMessagePrefix + "\n" + hex.EncodeToString(sum[:]) + "\n" + digest)
}

// Sign produces the detached base64 signature for a manifest and digest.
func Sign(priv ed25519.PrivateKey, manifestRaw []byte, digest string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignedMessage(manifestRaw, digest)))
}

// Verify checks sigB64 over the manifest and digest against the key the
// signature names. An empty or unknown key list rejects everything.
func Verify(keys []TrustedKey, keyID string, manifestRaw []byte, digest, sigB64 string) error {
	var key *TrustedKey
	for i := range keys {
		if keys[i].KeyID == keyID && keyID != "" {
			key = &keys[i]
			break
		}
	}
	if key == nil {
		return ErrUntrusted
	}
	pub, err := ParsePublicKey(key.PublicKey)
	if err != nil {
		return fmt.Errorf("%w: stored key is unusable", ErrUntrusted)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, SignedMessage(manifestRaw, digest), sig) {
		return ErrBadSignature
	}
	return nil
}
