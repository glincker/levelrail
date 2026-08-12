package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// EncryptValue encrypts plaintext under dek (a raw, unwrapped data
// encryption key from GenerateDEK or UnwrapDEK) using AES-256-GCM. The
// returned bytes are nonce-prefixed ciphertext, self-contained: nothing
// else needs to be stored alongside it to decrypt later, other than the
// dek itself.
//
// AES-GCM, not age directly, for actual secret values: DEKs are meant to
// be reused across every secret value for one app (that's the point of
// the envelope, wrap the DEK once, encrypt many values fast with it),
// and symmetric AES-GCM is the right tool for repeated same-key
// encryption; age's own asymmetric machinery is reserved for the one
// wrap operation per DEK in dek.go.
func EncryptValue(dek []byte, plaintext string) ([]byte, error) {
	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// DecryptValue reverses EncryptValue.
func DecryptValue(dek []byte, ciphertext []byte) (string, error) {
	gcm, err := newGCM(dek)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("secrets: ciphertext too short to contain a nonce")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt: %w", err)
	}
	return string(plaintext), nil
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
