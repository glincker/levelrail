package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

const keyFileFormat = "levelrail-announce-key-v1"

// KeyFile is the on disk private key. It must only ever live offline.
type KeyFile struct {
	Format     string `json:"format"`
	KeyID      string `json:"key_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

type RoleKey struct {
	KeyID     string   `json:"key_id"`
	PublicKey string   `json:"public_key"`
	Tiers     []string `json:"tiers"`
}

// Roles is the public key list with the tiers each key may sign. The
// instance embeds the same data.
type Roles struct {
	Keys    []RoleKey `json:"keys"`
	Revoked []string  `json:"revoked,omitempty"`
}

type SigEntry struct {
	KeyID string `json:"key_id"`
	Sig   string `json:"sig"`
}

func fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newKeyFile(id string, rnd io.Reader) (*KeyFile, error) {
	if !keyIDRe.MatchString(id) {
		return nil, fmt.Errorf("key id must match %s", keyIDRe)
	}
	pub, priv, err := ed25519.GenerateKey(rnd)
	if err != nil {
		return nil, err
	}
	return &KeyFile{
		Format:     keyFileFormat,
		KeyID:      id,
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: base64.StdEncoding.EncodeToString(priv.Seed()),
	}, nil
}

// writeKeyFile refuses to overwrite and creates the file 0600.
func writeKeyFile(path string, k *KeyFile) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists, refusing to overwrite", path)
		}
		return err
	}
	raw, _ := json.MarshalIndent(k, "", "  ")
	if _, err := f.Write(append(raw, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func loadKeyFile(path string) (*KeyFile, ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var k KeyFile
	if err := json.Unmarshal(raw, &k); err != nil || k.Format != keyFileFormat {
		return nil, nil, fmt.Errorf("%s is not a %s file", path, keyFileFormat)
	}
	seed, err := base64.StdEncoding.DecodeString(k.PrivateKey)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("%s has a bad private key", path)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)) != k.PublicKey {
		return nil, nil, fmt.Errorf("%s public key does not match private key", path)
	}
	return &k, priv, nil
}

func loadRoles(path string) (*Roles, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Roles
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("roles: %w", err)
	}
	seen := map[string]bool{}
	for _, k := range r.Keys {
		pub, derr := base64.StdEncoding.DecodeString(k.PublicKey)
		switch {
		case !keyIDRe.MatchString(k.KeyID):
			return nil, fmt.Errorf("roles: bad key id %q", k.KeyID)
		case seen[k.KeyID]:
			return nil, fmt.Errorf("roles: duplicate key id %q", k.KeyID)
		case derr != nil || len(pub) != ed25519.PublicKeySize:
			return nil, fmt.Errorf("roles: key %q has no valid public key", k.KeyID)
		}
		for _, t := range k.Tiers {
			if !validTiers[t] {
				return nil, fmt.Errorf("roles: key %q has unknown tier %q", k.KeyID, t)
			}
		}
		seen[k.KeyID] = true
	}
	return &r, nil
}

func (r *Roles) key(id string) *RoleKey {
	for i := range r.Keys {
		if r.Keys[i].KeyID == id {
			return &r.Keys[i]
		}
	}
	return nil
}

func (r *Roles) revoked(id string, extra map[string]bool) bool {
	return slices.Contains(r.Revoked, id) || extra[id]
}

func randReader() io.Reader { return rand.Reader }
