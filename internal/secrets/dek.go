package secrets

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"

	"filippo.io/age"
)

// dekSize is 32 bytes: AES-256's key size, what EncryptValue/
// DecryptValue in value.go expect.
const dekSize = 32

// WrappedDEK is a per-app data encryption key, encrypted under a
// MasterKey. Safe to persist at rest, useless without the matching
// MasterKey to unwrap it, which is the entire point of wrapping it
// rather than storing the raw key.
type WrappedDEK []byte

// GenerateDEK creates a new random data encryption key and wraps it
// under mk. The raw key is returned for immediate use, encrypting
// values right away, and must never be persisted itself, only the
// wrapped form returned alongside it.
func (mk *MasterKey) GenerateDEK() (raw []byte, wrapped WrappedDEK, err error) {
	raw = make([]byte, dekSize)
	if _, err := rand.Read(raw); err != nil {
		return nil, nil, fmt.Errorf("secrets: generate dek: %w", err)
	}

	wrapped, err = mk.wrap(raw)
	if err != nil {
		return nil, nil, err
	}
	return raw, wrapped, nil
}

// UnwrapDEK decrypts a WrappedDEK back to its raw form, for decrypting
// values that were encrypted under it.
func (mk *MasterKey) UnwrapDEK(wrapped WrappedDEK) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(wrapped), mk.identity)
	if err != nil {
		return nil, fmt.Errorf("secrets: unwrap dek: %w", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("secrets: read unwrapped dek: %w", err)
	}
	if len(raw) != dekSize {
		return nil, fmt.Errorf("secrets: unwrapped dek has unexpected length %d, want %d", len(raw), dekSize)
	}
	return raw, nil
}

func (mk *MasterKey) wrap(raw []byte) (WrappedDEK, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, mk.recipient)
	if err != nil {
		return nil, fmt.Errorf("secrets: wrap dek: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return nil, fmt.Errorf("secrets: wrap dek: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("secrets: wrap dek: close: %w", err)
	}
	return WrappedDEK(buf.Bytes()), nil
}
