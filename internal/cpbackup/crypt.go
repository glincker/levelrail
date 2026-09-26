package cpbackup

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// ErrNoRecipients means an encryption was asked for without any recipient.
var ErrNoRecipients = errors.New("at least one age recipient (public key) is required")

// ErrWrongIdentity means no supplied identity can decrypt the file.
var ErrWrongIdentity = errors.New("none of the supplied identities can decrypt this backup")

// ParseRecipients parses age public keys, X25519 (age1...) or hybrid (age1pq1...).
func ParseRecipients(keys []string) ([]age.Recipient, error) {
	if len(keys) == 0 {
		return nil, ErrNoRecipients
	}
	out := make([]age.Recipient, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if strings.HasPrefix(k, "AGE-SECRET-KEY") {
			return nil, errors.New("a private key was supplied where a public recipient is required")
		}
		if r, err := age.ParseX25519Recipient(k); err == nil {
			out = append(out, r)
			continue
		}
		r, err := age.ParseHybridRecipient(k)
		if err != nil {
			return nil, fmt.Errorf("invalid age recipient %q", truncateKey(k))
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, ErrNoRecipients
	}
	return out, nil
}

func truncateKey(k string) string {
	if len(k) > 16 {
		return k[:16] + "..."
	}
	return k
}

// ParseIdentityFile parses an age identity file (one key per line, comments allowed).
func ParseIdentityFile(path string) ([]age.Identity, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied identity path
	if err != nil {
		return nil, fmt.Errorf("open identity file: %w", err)
	}
	defer func() { _ = f.Close() }()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("parse identity file: %w", err)
	}
	if len(ids) == 0 {
		return nil, errors.New("identity file holds no keys")
	}
	return ids, nil
}

// Identity is a freshly generated age keypair.
type Identity struct {
	// Recipient is the public key, safe to store and share.
	Recipient string
	// Secret is the private key line. Never log it.
	Secret string
}

// GenerateIdentity makes an X25519 keypair, or a hybrid post-quantum one when hybrid is set.
func GenerateIdentity(hybrid bool) (Identity, error) {
	if hybrid {
		id, err := age.GenerateHybridIdentity()
		if err != nil {
			return Identity{}, fmt.Errorf("generate hybrid identity: %w", err)
		}
		return Identity{Recipient: id.Recipient().String(), Secret: id.String()}, nil
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return Identity{}, fmt.Errorf("generate identity: %w", err)
	}
	return Identity{Recipient: id.Recipient().String(), Secret: id.String()}, nil
}

// RecipientOf derives the public recipient string from a parsed identity, when it has one.
func RecipientOf(id age.Identity) (string, bool) {
	switch v := id.(type) {
	case *age.X25519Identity:
		return v.Recipient().String(), true
	case *age.HybridIdentity:
		return v.Recipient().String(), true
	}
	return "", false
}

type sealed struct {
	Size        int64
	SHA256      string
	PlainSize   int64
	PlainSHA256 string
}

type hashCounter struct {
	h hash.Hash
	n int64
}

func (c *hashCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return c.h.Write(p)
}

func newHashCounter() *hashCounter { return &hashCounter{h: sha256.New()} }

func (c *hashCounter) sum() string { return hex.EncodeToString(c.h.Sum(nil)) }

// seal gzips plainPath and encrypts it to recipients at outPath, hashing both sides.
func seal(plainPath, outPath string, recipients []age.Recipient) (sealed, error) {
	in, err := os.Open(plainPath) //nolint:gosec // path built by this package
	if err != nil {
		return sealed{}, fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path built by this package
	if err != nil {
		return sealed{}, fmt.Errorf("create encrypted backup: %w", err)
	}
	cipherHash, plainHash := newHashCounter(), newHashCounter()
	enc, err := age.Encrypt(io.MultiWriter(out, cipherHash), recipients...)
	if err != nil {
		_ = out.Close()
		return sealed{}, fmt.Errorf("start encryption: %w", err)
	}
	gz := gzip.NewWriter(enc)
	if _, err := io.Copy(io.MultiWriter(gz, plainHash), in); err != nil {
		_ = out.Close()
		return sealed{}, fmt.Errorf("encrypt snapshot: %w", err)
	}
	for _, closer := range []io.Closer{gz, enc} {
		if err := closer.Close(); err != nil {
			_ = out.Close()
			return sealed{}, fmt.Errorf("finish encryption: %w", err)
		}
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return sealed{}, fmt.Errorf("sync encrypted backup: %w", err)
	}
	if err := out.Close(); err != nil {
		return sealed{}, fmt.Errorf("close encrypted backup: %w", err)
	}
	return sealed{Size: cipherHash.n, SHA256: cipherHash.sum(), PlainSize: plainHash.n, PlainSHA256: plainHash.sum()}, nil
}

// open decrypts and gunzips cipherPath to outPath, refusing to write more than maxPlain bytes.
func open(cipherPath, outPath string, identities []age.Identity, maxPlain int64) (sealed, error) {
	in, err := os.Open(cipherPath) //nolint:gosec // path built by this package
	if err != nil {
		return sealed{}, fmt.Errorf("open encrypted backup: %w", err)
	}
	defer func() { _ = in.Close() }()
	dec, err := age.Decrypt(in, identities...)
	if err != nil {
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return sealed{}, ErrWrongIdentity
		}
		return sealed{}, fmt.Errorf("decrypt backup: %w", err)
	}
	gz, err := gzip.NewReader(dec)
	if err != nil {
		return sealed{}, fmt.Errorf("decompress backup: %w", err)
	}
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path built by this package
	if err != nil {
		return sealed{}, fmt.Errorf("create restored database: %w", err)
	}
	plainHash := newHashCounter()
	n, err := io.Copy(io.MultiWriter(out, plainHash), io.LimitReader(gz, maxPlain+1))
	if err != nil {
		_ = out.Close()
		return sealed{}, fmt.Errorf("decrypt backup: %w", err)
	}
	if n > maxPlain {
		_ = out.Close()
		return sealed{}, errors.New("decrypted backup is larger than its manifest declares")
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return sealed{}, fmt.Errorf("sync restored database: %w", err)
	}
	if err := out.Close(); err != nil {
		return sealed{}, fmt.Errorf("close restored database: %w", err)
	}
	return sealed{PlainSize: n, PlainSHA256: plainHash.sum()}, nil
}
