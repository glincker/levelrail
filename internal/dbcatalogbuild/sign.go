package dbcatalogbuild

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// GenerateKey returns a base64 private key and public key pair.
func GenerateKey() (priv, pub string, err error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("dbcatalogbuild: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sk), base64.StdEncoding.EncodeToString(pk), nil
}

// Sign returns the base64 ed25519 signature of data.
func Sign(privB64 string, data []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privB64))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return "", errors.New("dbcatalogbuild: signing key is not a base64 ed25519 private key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(raw), data)), nil
}

// Verify checks a base64 signature of data against a base64 public key.
func Verify(pubB64, sigB64 string, data []byte) error {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("dbcatalogbuild: public key is not a base64 ed25519 key")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return errors.New("dbcatalogbuild: signature is not base64")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return errors.New("dbcatalogbuild: signature does not match")
	}
	return nil
}
