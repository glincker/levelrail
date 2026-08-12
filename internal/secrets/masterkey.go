// Package secrets implements CLAUDE.md 4.10's envelope encryption:
// per-app data encryption keys, wrapped by a master key held only by the
// control plane, using filippo.io/age for the crypto primitives.
//
// This is the primitives layer only: MasterKey, DEK wrap/unwrap, and
// value encrypt/decrypt, standalone and fully tested, the same shape
// internal/build and internal/ingress were built as Phase 0 spikes
// before being wired into the real deploy flow. Wiring this into
// internal/deploy's env resolution and internal/reconcile/application's
// container creation (decrypting a value only at container-create time,
// never persisting it, per CLAUDE.md 4.10) is deliberate follow-up work,
// not built here: those integration points were being actively modified
// by parallel work when this package was built, and a real per-app DEK
// needs its own reviewed store schema (CLAUDE.md 8: schema changes need
// one coherent session), not one bolted on alongside unrelated changes.
package secrets

import (
	"fmt"

	"filippo.io/age"
)

// MasterKey is the root key this control plane holds. It is never used
// to encrypt secret values directly, only to wrap and unwrap per-app
// data encryption keys (see DEK in dek.go). Uses age's Hybrid key type:
// per the age library's own current documentation, "the standard age
// private/public key" as of this version, safe against future
// cryptographically-relevant quantum computers.
type MasterKey struct {
	identity  *age.HybridIdentity
	recipient *age.HybridRecipient
}

// GenerateMasterKey creates a new master key. Used for bootstrapping a
// fresh install and in tests. A real deployment persists the resulting
// String() output (see LoadMasterKey) rather than regenerating on every
// start, generating a new key would make every previously-wrapped DEK
// permanently unwrappable.
func GenerateMasterKey() (*MasterKey, error) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		return nil, fmt.Errorf("secrets: generate master key: %w", err)
	}
	return &MasterKey{identity: id, recipient: id.Recipient()}, nil
}

// LoadMasterKey parses a master key from its serialized identity string
// (MasterKey.String()'s output). CLAUDE.md 4.10: "Master key can be
// sourced from file, env, or an external KMS interface added later."
// This function only handles the parsing; sourcing the string itself
// (reading a file, reading an env var) is the caller's job, matching
// internal/brand.Load's own "load from a path the caller resolved"
// shape rather than this package reaching into the environment itself.
func LoadMasterKey(serialized string) (*MasterKey, error) {
	id, err := age.ParseHybridIdentity(serialized)
	if err != nil {
		return nil, fmt.Errorf("secrets: parse master key: %w", err)
	}
	return &MasterKey{identity: id, recipient: id.Recipient()}, nil
}

// String returns the master key's serialized identity, for persisting
// wherever LoadMasterKey will later read it back from. Treat the result
// as exactly as sensitive as any other secret: never log it, never write
// it anywhere other than the deliberately chosen key storage location.
func (mk *MasterKey) String() string {
	return mk.identity.String()
}
