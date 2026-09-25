package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Binding is the storage slot a ciphertext belongs to. It is sealed
// inside the ciphertext and checked on every decrypt, so a ciphertext
// copied into a different slot fails closed instead of decrypting.
type Binding struct {
	Scope string
	Owner string
	Key   string
}

var (
	// ErrBindingMismatch means a ciphertext decrypted fine but was sealed
	// for a different slot than the one it was read from.
	ErrBindingMismatch = errors.New("secrets: ciphertext is bound to a different slot")
	// ErrMalformedEnvelope means a bound ciphertext authenticated but its
	// framing did not parse.
	ErrMalformedEnvelope = errors.New("secrets: malformed secret envelope")
	// ErrInvalidBinding means a Binding has an empty or oversized field.
	ErrInvalidBinding = errors.New("secrets: invalid binding")
)

// envelopeV1 prefixes every bound ciphertext and is also its GCM
// associated data, so stripping it to pass the rest off as legacy fails
// authentication instead of decrypting.
var envelopeV1 = []byte{0xB7, 0x1C, 0xE5, 0x5A, 0x01}

// hasBoundPrefix reports whether ciphertext carries the bound envelope
// prefix. A cheap format check only; DecryptValue is authoritative.
func hasBoundPrefix(ciphertext []byte) bool {
	return bytes.HasPrefix(ciphertext, envelopeV1)
}

// boundPrefix returns a copy of the bound envelope prefix, for storage
// queries that count formats without decrypting.
func boundPrefix() []byte {
	return bytes.Clone(envelopeV1)
}

func (b Binding) validate() error {
	for _, f := range []string{b.Scope, b.Owner, b.Key} {
		if f == "" || len(f) > math.MaxUint16 {
			return ErrInvalidBinding
		}
	}
	return nil
}

// encodeHeader frames the binding as three uint16 length-prefixed fields.
func (b Binding) encodeHeader() []byte {
	out := make([]byte, 0, 6+len(b.Scope)+len(b.Owner)+len(b.Key))
	for _, f := range []string{b.Scope, b.Owner, b.Key} {
		out = binary.BigEndian.AppendUint16(out, uint16(len(f))) //nolint:gosec // validate caps every field at MaxUint16
		out = append(out, f...)
	}
	return out
}

// splitFrame returns the length of the binding header at the start of
// frame and the value that follows it.
func splitFrame(frame []byte) (headerLen int, value []byte, err error) {
	off := 0
	for range 3 {
		if len(frame)-off < 2 {
			return 0, nil, ErrMalformedEnvelope
		}
		n := int(binary.BigEndian.Uint16(frame[off:]))
		off += 2
		if n == 0 || len(frame)-off < n {
			return 0, nil, ErrMalformedEnvelope
		}
		off += n
	}
	return off, frame[off:], nil
}

// EncryptValue encrypts plaintext under dek (a raw, unwrapped DEK) with
// AES-256-GCM, sealing b inside so the result only decrypts back through
// DecryptValue with the same Binding.
func EncryptValue(dek []byte, b Binding, plaintext string) ([]byte, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: generate nonce: %w", err)
	}

	frame := append(b.encodeHeader(), plaintext...)
	out := make([]byte, 0, len(envelopeV1)+len(nonce)+len(frame)+gcm.Overhead())
	out = append(out, envelopeV1...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, frame, envelopeV1), nil
}

// DecryptValue reverses EncryptValue, failing with ErrBindingMismatch
// when the ciphertext was sealed for a different Binding. legacy is true
// for a ciphertext written before bindings existed, which carries no
// slot to check.
func DecryptValue(dek []byte, b Binding, ciphertext []byte) (plaintext string, legacy bool, err error) {
	if err := b.validate(); err != nil {
		return "", false, err
	}
	gcm, err := newGCM(dek)
	if err != nil {
		return "", false, err
	}

	if hasBoundPrefix(ciphertext) {
		frame, boundErr := openGCM(gcm, ciphertext[len(envelopeV1):], envelopeV1)
		if boundErr == nil {
			value, err := checkBinding(frame, b)
			return value, false, err
		}
		// A legacy nonce starts with the prefix by chance about once in 2^40.
		if pt, legacyErr := openGCM(gcm, ciphertext, nil); legacyErr == nil {
			return string(pt), true, nil
		}
		return "", false, boundErr
	}

	pt, err := openGCM(gcm, ciphertext, nil)
	if err != nil {
		return "", false, err
	}
	return string(pt), true, nil
}

func checkBinding(frame []byte, want Binding) (string, error) {
	headerLen, value, err := splitFrame(frame)
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare(frame[:headerLen], want.encodeHeader()) != 1 {
		return "", ErrBindingMismatch
	}
	return string(value), nil
}

func openGCM(gcm cipher.AEAD, ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("secrets: ciphertext too short to contain a nonce")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt: %w", err)
	}
	return pt, nil
}

func newGCM(dek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("secrets: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: new gcm: %w", err)
	}
	return gcm, nil
}
