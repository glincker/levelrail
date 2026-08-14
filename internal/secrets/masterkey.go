// Package secrets implements envelope encryption: per-app data
// encryption keys, wrapped by a master key held only by the control
// plane, using filippo.io/age for the crypto primitives.
//
// masterkey.go, dek.go, and value.go are the primitives layer: MasterKey,
// DEK wrap/unwrap, and value encrypt/decrypt, standalone and fully
// tested, knowing nothing about storage. manager.go (TASKS.md 1.7's
// follow-up, landed 2026-08-12) is the integration layer: Manager
// combines a MasterKey with internal/store's new service_secrets/
// service_secret_values tables to generate-or-reuse a per-app DEK,
// encrypt and persist a value, and decrypt one back. Callers:
// internal/deploy.Pipeline checks whether a { secret: true } var has a
// value before deploying (never the value itself), and
// internal/reconcile/application.Controller decrypts a value only
// immediately before docker.Runtime.Create, never persisting it, which
// is exactly the sequencing envelope encryption is meant to guarantee.
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
// (MasterKey.String()'s output). The master key can be sourced from a
// file, an env var, or an external KMS interface added later.
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
