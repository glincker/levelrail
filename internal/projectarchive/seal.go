package projectarchive

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode/utf8"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
)

// EnvMinPassphrase names the env var that can raise the shortest accepted
// passphrase. It cannot lower it below minPassphraseFloor.
const EnvMinPassphrase = "APP_PROJECT_ARCHIVE_MIN_PASSPHRASE" //nolint:gosec // an env var name, not a credential

const (
	minPassphraseFloor = 12
	defaultKDFLogN     = 17
	maxKDFLogN         = 20
	kdfR               = 8
	kdfP               = 1
	macLen             = 32
	saltLen            = 16
)

// magic, log2 N, r, p, salt, nonce, manifest mac, ciphertext.
var sealMagic = []byte("LRXS2")

// ErrWrongPassphrase covers a wrong passphrase and a damaged secrets blob,
// which AES-GCM cannot tell apart.
var ErrWrongPassphrase = errors.New("projectarchive: wrong passphrase or damaged secrets")

// ErrManifestMismatch means the sealed secrets do not belong to this manifest.
var ErrManifestMismatch = errors.New("projectarchive: wrong passphrase, or the sealed secrets do not belong to this archive")

// MinPassphrase is the shortest passphrase, in characters, Seal accepts.
func MinPassphrase() int {
	if v, err := strconv.Atoi(os.Getenv(EnvMinPassphrase)); err == nil && v > minPassphraseFloor {
		return v
	}
	return minPassphraseFloor
}

// PassphraseOK reports whether pass is long enough, counted in characters.
func PassphraseOK(pass string) bool { return utf8.RuneCountInString(pass) >= MinPassphrase() }

type kdfParams struct{ logN, r, p byte }

// deriveKeys runs scrypt once and splits the output with HKDF into an
// encryption key and an independent manifest MAC key.
func deriveKeys(pass string, salt []byte, k kdfParams) (encKey, macKey []byte, err error) {
	if k.logN < 10 || k.logN > maxKDFLogN || k.r == 0 || k.p == 0 {
		return nil, nil, ErrFormat
	}
	root, err := scrypt.Key([]byte(pass), salt, 1<<k.logN, int(k.r), int(k.p), 32)
	if err != nil {
		return nil, nil, err
	}
	encKey, macKey = make([]byte, 32), make([]byte, 32)
	if _, err := io.ReadFull(hkdf.Expand(sha256.New, root, []byte("lrx enc")), encKey); err != nil {
		return nil, nil, err
	}
	if _, err := io.ReadFull(hkdf.Expand(sha256.New, root, []byte("lrx manifest mac")), macKey); err != nil {
		return nil, nil, err
	}
	return encKey, macKey, nil
}

func manifestMAC(macKey, manifest []byte) []byte {
	h := hmac.New(sha256.New, macKey)
	h.Write(manifest)
	return h.Sum(nil)
}

// seal encrypts plain under a key derived from pass and binds it to the
// canonical manifest through an HMAC that also sits in the GCM AAD.
func seal(pass string, plain, manifest []byte) ([]byte, error) {
	if !PassphraseOK(pass) {
		return nil, fmt.Errorf("projectarchive: the passphrase needs at least %d characters", MinPassphrase())
	}
	k := kdfParams{defaultKDFLogN, kdfR, kdfP}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	encKey, macKey, err := deriveKeys(pass, salt, k)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(encKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	hdr := append(append(append(append([]byte{}, sealMagic...), k.logN, k.r, k.p), salt...), nonce...)
	hdr = append(hdr, manifestMAC(macKey, manifest)...)
	return gcm.Seal(hdr, nonce, plain, hdr), nil
}

// open verifies the manifest binding before it decrypts anything.
func open(pass string, blob, manifest []byte) ([]byte, error) {
	const fixed = 5 + 3 + saltLen
	hdrLen := fixed + 12 + macLen
	if len(blob) < hdrLen+16 || !bytes.Equal(blob[:len(sealMagic)], sealMagic) {
		return nil, ErrFormat
	}
	k := kdfParams{blob[5], blob[6], blob[7]}
	salt := blob[fixed-saltLen : fixed]
	nonce := blob[fixed : fixed+12]
	mac := blob[fixed+12 : hdrLen]
	encKey, macKey, err := deriveKeys(pass, salt, k)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(mac, manifestMAC(macKey, manifest)) {
		// A wrong passphrase lands here too, so the two cases stay one error for the caller.
		return nil, ErrManifestMismatch
	}
	gcm, err := newGCM(encKey)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, blob[hdrLen:], blob[:hdrLen])
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
