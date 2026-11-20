package projectarchive

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/crypto/scrypt"
)

// EnvMinPassphrase names the env var holding the shortest accepted passphrase.
const EnvMinPassphrase = "APP_PROJECT_ARCHIVE_MIN_PASSPHRASE" //nolint:gosec // an env var name, not a credential

var sealMagic = []byte("LRXS1")

// ErrWrongPassphrase covers a wrong passphrase and a damaged secrets blob,
// which AES-GCM cannot tell apart.
var ErrWrongPassphrase = errors.New("projectarchive: wrong passphrase or damaged secrets")

// MinPassphrase is the shortest passphrase Seal accepts.
func MinPassphrase() int {
	if v, err := strconv.Atoi(os.Getenv(EnvMinPassphrase)); err == nil && v > 0 {
		return v
	}
	return 12
}

func deriveKey(pass string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, 1<<15, 8, 1, 32)
}

// Seal encrypts plain under a key derived from pass with scrypt. The
// layout is magic, salt, nonce, ciphertext.
func Seal(pass string, plain []byte) ([]byte, error) {
	if len(pass) < MinPassphrase() {
		return nil, fmt.Errorf("projectarchive: the passphrase needs at least %d characters", MinPassphrase())
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := deriveKey(pass, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append(append(append([]byte{}, sealMagic...), salt...), nonce...)
	return gcm.Seal(out, nonce, plain, sealMagic), nil
}

// Open reverses Seal.
func Open(pass string, blob []byte) ([]byte, error) {
	if len(blob) < len(sealMagic)+16+12 || string(blob[:len(sealMagic)]) != string(sealMagic) {
		return nil, ErrFormat
	}
	salt := blob[len(sealMagic) : len(sealMagic)+16]
	key, err := deriveKey(pass, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	rest := blob[len(sealMagic)+16:]
	if len(rest) < gcm.NonceSize() {
		return nil, ErrFormat
	}
	plain, err := gcm.Open(nil, rest[:gcm.NonceSize()], rest[gcm.NonceSize():], sealMagic)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plain, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// SecretPayload is what secrets.enc holds once opened: stored secret values
// keyed "app/NAME", and ${{ env.NAME }} placeholder values keyed by name.
type SecretPayload struct {
	Secrets map[string]string `json:"secrets"`
	Vars    map[string]string `json:"vars,omitempty"`
}
