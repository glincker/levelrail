package network

// This file: loading and persisting a node's own WireGuard private key
// on local disk, factored out so cmd/levelrail-agent's own mesh setup
// (the node-side counterpart to cmd/levelrail/mesh.go's
// loadOrGenerateMeshKey/persistMeshKey, which predate this file and stay
// as they are rather than being rewired to it, a real, deliberate "don't
// touch what already works" tradeoff) gets an identical, tested
// implementation instead of a second hand-written copy of the same
// read-or-generate-then-persist logic landing on a different process's
// disk.

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateKey loads a private key from path, or generates and
// persists a fresh one if no file exists there yet. Stored as
// Key.String()'s base64 form, the same textual form this package ships
// over the wire and wg(8) itself prints, so the file is inspectable with
// nothing but `cat`.
func LoadOrGenerateKey(path string) (Key, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled data directory path, not user input
	if err == nil {
		key, parseErr := ParseKey(string(raw))
		if parseErr != nil {
			return Key{}, fmt.Errorf("network: parse persisted key at %q: %w", path, parseErr)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return Key{}, fmt.Errorf("network: read key file %q: %w", path, err)
	}

	key, err := GeneratePrivateKey()
	if err != nil {
		return Key{}, fmt.Errorf("network: generate key for %q: %w", path, err)
	}
	if err := PersistKey(path, key); err != nil {
		return Key{}, err
	}
	return key, nil
}

// PersistKey writes key to path, creating path's parent directory if
// needed. 0o600: this file holds private key material, the same
// permission every other persisted credential in this codebase uses
// (agent CA keys, node join tokens' plaintext-once form).
func PersistKey(path string, key Key) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // operator-controlled path, not user input
			return fmt.Errorf("network: create directory for key file %q: %w", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(key.String()), 0o600); err != nil { //nolint:gosec // operator-controlled path, not user input
		return fmt.Errorf("network: persist key to %q: %w", path, err)
	}
	return nil
}
