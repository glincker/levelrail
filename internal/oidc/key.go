// Package oidc mints short-lived, signed OIDC tokens for pipeline jobs
// that opt in with `oidc: {audience: ...}`, and publishes the matching
// JWKS so AWS IAM, GCP workload identity federation, and Vault's JWT
// auth method can verify them without a long-lived credential ever
// touching a job. Self-contained: it never imports internal/secrets or
// internal/pipeline, only the narrow KeyStore interface a caller
// satisfies (internal/secrets.Manager does, structurally), the same
// consumer-defined-boundary shape the rest of this codebase uses for
// DockerPinger and friends.
//
// The signing key is ECDSA P-256 (ES256), generated once and persisted
// through the caller's KeyStore, deliberately not the envelope
// encryption master key (internal/secrets): that key is an age Hybrid
// key meant for wrapping data encryption keys, not for producing a
// signature an external verifier can check against a published public
// key. This is a dedicated key for this one purpose.
package oidc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
)

// Key is one ES256 signing key, identified by KID.
type Key struct {
	kid     string
	private *ecdsa.PrivateKey
}

// generateKey creates a fresh P-256 key pair. KID is derived from the
// public key itself (a hash of its uncompressed point), not randomly
// generated, so re-deriving it from a reloaded key is always consistent.
func generateKey() (*Key, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("oidc: generate signing key: %w", err)
	}
	return &Key{kid: kidFor(&priv.PublicKey), private: priv}, nil
}

func kidFor(pub *ecdsa.PublicKey) string {
	sum := sha256.Sum256(elliptic.Marshal(elliptic.P256(), pub.X, pub.Y)) //nolint:staticcheck // SEC1 uncompressed point encoding, fine for a key-identifier hash input
	return hex.EncodeToString(sum[:])[:16]
}

// pemEncode returns the key's PKCS#8 private key, PEM-encoded, for
// persisting through the caller's KeyStore.
func (k *Key) pemEncode() (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(k.private)
	if err != nil {
		return "", fmt.Errorf("oidc: marshal signing key: %w", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

// parseKeyPEM reloads a key persisted by pemEncode. kid is trusted from
// the caller's own stored record rather than re-derived, so a future kid
// derivation change can't silently invalidate an already-issued token's
// verification against a cached JWKS.
func parseKeyPEM(kid, pemStr string) (*Key, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("oidc: no PEM block found in signing key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oidc: parse signing key: %w", err)
	}
	priv, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("oidc: signing key is not ECDSA")
	}
	return &Key{kid: kid, private: priv}, nil
}

// JWK is one entry in a JWKS document: the public half of an EC signing
// key, RFC 7518 §6.2's field names.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

// JWKS is a JSON Web Key Set: RFC 7517's document shape.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// jwk returns this key's public JWK.
func (k *Key) jwk() JWK {
	size := (k.private.Curve.Params().BitSize + 7) / 8
	return JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(k.private.X.FillBytes(make([]byte, size))),
		Y:   base64.RawURLEncoding.EncodeToString(k.private.Y.FillBytes(make([]byte, size))),
		Kid: k.kid,
		Use: "sig",
		Alg: "ES256",
	}
}
