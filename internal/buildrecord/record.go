// Package buildrecord signs and verifies the record the control plane keeps for
// an image a remote builder produced, so a runner can trust what it deploys.
package buildrecord

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

const (
	legacyKeyService = "build-records"
	keyName          = "signing-key"
	domain           = "levelrail-build-record-v1\n"
)

var keyService = secrets.ControlPlaneService("build-records")

func validSeed(v string) bool {
	raw, err := base64.StdEncoding.DecodeString(v)
	return err == nil && len(raw) == ed25519.SeedSize
}

// ErrBadSignature means the record was altered or signed by another key.
var ErrBadSignature = errors.New("buildrecord: signature does not verify")

// Claims is the signed content. Field order is part of the format.
type Claims struct {
	App           string    `json:"app"`
	Environment   string    `json:"environment"`
	Repo          string    `json:"repo"`
	Digest        string    `json:"digest"`
	BuilderNodeID string    `json:"builder_node_id"`
	BuilderTrust  string    `json:"builder_trust"`
	SourceRef     string    `json:"source_ref"`
	IssuedAt      time.Time `json:"issued_at"`
	Nonce         string    `json:"nonce"`
}

// Signed is Claims plus the control plane signature and the id of the key used.
type Signed struct {
	Claims    Claims `json:"claims"`
	Signature string `json:"signature"`
	KeyID     string `json:"key_id"`
}

// KeyStore is the slice of the secrets manager the signing key needs.
type KeyStore interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// Signer signs build records with the control plane ed25519 key.
type Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// NewSigner wraps an ed25519 private key.
func NewSigner(priv ed25519.PrivateKey) *Signer {
	return &Signer{priv: priv, pub: priv.Public().(ed25519.PublicKey)}
}

// LoadOrCreateSigner reads the key from the secrets store, generating it on first use.
// The seed is stored encrypted and never logged.
func LoadOrCreateSigner(ctx context.Context, ks KeyStore) (*Signer, error) {
	if km, ok := ks.(secrets.KeyMover); ok {
		if err := secrets.MigrateKey(ctx, km, legacyKeyService, keyService, keyName, validSeed, nil); err != nil {
			return nil, fmt.Errorf("buildrecord: migrate signing key: %w", err)
		}
	}
	seed, err := ks.Resolve(ctx, keyService, keyName)
	if err == nil {
		raw, derr := base64.StdEncoding.DecodeString(seed)
		if derr != nil || len(raw) != ed25519.SeedSize {
			return nil, errors.New("buildrecord: stored signing key is malformed")
		}
		return NewSigner(ed25519.NewKeyFromSeed(raw)), nil
	}
	if !errors.Is(err, secrets.ErrValueNotFound) {
		return nil, fmt.Errorf("buildrecord: load signing key: %w", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("buildrecord: generate signing key: %w", err)
	}
	if err := ks.SetValue(ctx, keyService, keyName, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
		return nil, fmt.Errorf("buildrecord: store signing key: %w", err)
	}
	return NewSigner(priv), nil
}

// KeyID is a short stable fingerprint of the public key.
func (s *Signer) KeyID() string {
	sum := sha256.Sum256(s.pub)
	return hex.EncodeToString(sum[:8])
}

// PublicKey is the verification key.
func (s *Signer) PublicKey() ed25519.PublicKey { return s.pub }

func payload(c Claims) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("buildrecord: encode claims: %w", err)
	}
	return append([]byte(domain), b...), nil
}

// Sign stamps the claims with a fresh nonce (when empty) and signs them.
func (s *Signer) Sign(c Claims) (Signed, error) {
	if c.Nonce == "" {
		var n [16]byte
		if _, err := rand.Read(n[:]); err != nil {
			return Signed{}, fmt.Errorf("buildrecord: nonce: %w", err)
		}
		c.Nonce = hex.EncodeToString(n[:])
	}
	msg, err := payload(c)
	if err != nil {
		return Signed{}, err
	}
	return Signed{Claims: c, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, msg)), KeyID: s.KeyID()}, nil
}

// Verify checks the signature against the signer's own public key.
func (s *Signer) Verify(r Signed) error { return Verify(s.pub, r) }

// Verify checks r against pub, failing closed on any decode problem.
func Verify(pub ed25519.PublicKey, r Signed) error {
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	msg, err := payload(r.Claims)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, sig) {
		return ErrBadSignature
	}
	return nil
}
