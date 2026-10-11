package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

// Backup object codecs recorded on store.BackupHistory.Codec. The empty
// string is a raw legacy object.
const (
	CodecZstd    = "zstd"
	CodecZstdAge = "zstd+age"
)

// Environment variables that tune sealed volume backups.
const (
	EnvBackupEncryption    = "APP_BACKUP_ENCRYPTION"
	EnvBackupEncryptionKey = "APP_BACKUP_ENCRYPTION_KEY_FILE"
	EnvBackupMaxSizeMB     = "APP_BACKUP_MAX_SIZE_MB"
	EnvBackupTimeoutMin    = "APP_BACKUP_TIMEOUT_MINUTES"
	EnvBackupUploadTries   = "APP_BACKUP_UPLOAD_ATTEMPTS"
	EnvBackupDrillInterval = "APP_BACKUP_DRILL_INTERVAL_HOURS"
	EnvBackupStaleMinutes  = "APP_BACKUP_STALE_RUNNING_MINUTES"
)

// ErrNoDecryptionKey means an encrypted object was opened without identities.
var ErrNoDecryptionKey = errors.New("backup is encrypted and this control plane has no decryption key")

// Sealer compresses and optionally encrypts a stream. A nil Sealer means the
// legacy raw encoding.
type Sealer struct {
	Recipients []age.Recipient
	Identities []age.Identity
}

// Codec is the codec name this Sealer produces.
func (s *Sealer) Codec() string {
	if s == nil {
		return ""
	}
	if len(s.Recipients) > 0 {
		return CodecZstdAge
	}
	return CodecZstd
}

// CodecSuffix is the object key suffix for the codec, tar payload assumed.
func CodecSuffix(codec string) string {
	switch codec {
	case CodecZstd:
		return ".zst"
	case CodecZstdAge:
		return ".zst.age"
	}
	return ""
}

// Seal returns a reader producing src compressed (and encrypted when
// recipients are set). Closing it early stops the pipeline.
func (s *Sealer) Seal(src io.Reader) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		var out io.Writer = pw
		var aw io.WriteCloser
		if s != nil && len(s.Recipients) > 0 {
			w, err := age.Encrypt(pw, s.Recipients...)
			if err != nil {
				_ = pw.CloseWithError(fmt.Errorf("start encryption: %w", err))
				return
			}
			aw = w
			out = w
		}
		zw, err := zstd.NewWriter(out, zstd.WithEncoderConcurrency(2))
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("start compression: %w", err))
			return
		}
		if _, err := io.Copy(zw, src); err != nil {
			_ = zw.Close()
			_ = pw.CloseWithError(fmt.Errorf("compress: %w", err))
			return
		}
		if err := zw.Close(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("finish compression: %w", err))
			return
		}
		if aw != nil {
			if err := aw.Close(); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("finish encryption: %w", err))
				return
			}
		}
		_ = pw.Close()
	}()
	return pr
}

// OpenSealed reverses Seal for the recorded codec. An empty codec returns r
// unchanged. The returned reader fails on a truncated or tampered object.
func OpenSealed(r io.Reader, codec string, identities []age.Identity) (io.ReadCloser, error) {
	switch codec {
	case "":
		return io.NopCloser(r), nil
	case CodecZstd:
		return newZstdReader(r)
	case CodecZstdAge:
		if len(identities) == 0 {
			return nil, ErrNoDecryptionKey
		}
		dr, err := age.Decrypt(r, identities...)
		if err != nil {
			return nil, fmt.Errorf("decrypt backup: %w", err)
		}
		return newZstdReader(dr)
	}
	return nil, fmt.Errorf("unknown backup codec %q", codec)
}

type zstdReadCloser struct{ *zstd.Decoder }

func (z zstdReadCloser) Close() error { z.Decoder.Close(); return nil }

func newZstdReader(r io.Reader) (io.ReadCloser, error) {
	d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, fmt.Errorf("start decompression: %w", err)
	}
	return zstdReadCloser{d}, nil
}

// LoadSealer builds the Sealer from the environment. Encryption is on unless
// APP_BACKUP_ENCRYPTION is "off"; the age identity lives in
// APP_BACKUP_ENCRYPTION_KEY_FILE, default <dataDir>/backup-encryption.key, and
// is generated on first use. An empty dataDir with no key file disables
// encryption.
func LoadSealer(lookup func(string) (string, bool), dataDir string) (*Sealer, error) {
	if v, ok := lookup(EnvBackupEncryption); ok && strings.EqualFold(strings.TrimSpace(v), "off") {
		return &Sealer{}, nil
	}
	path, _ := lookup(EnvBackupEncryptionKey)
	path = strings.TrimSpace(path)
	if path == "" {
		if dataDir == "" {
			return &Sealer{}, nil
		}
		path = filepath.Join(dataDir, "backup-encryption.key")
	}
	id, err := loadOrCreateIdentity(path)
	if err != nil {
		return nil, err
	}
	return &Sealer{Recipients: []age.Recipient{id.Recipient()}, Identities: []age.Identity{id}}, nil
}

func loadOrCreateIdentity(path string) (*age.X25519Identity, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-configured key path
	if err == nil {
		ids, perr := age.ParseIdentities(strings.NewReader(string(raw)))
		if perr != nil || len(ids) == 0 {
			return nil, fmt.Errorf("parse backup encryption key %q: %v", path, perr)
		}
		x, ok := ids[0].(*age.X25519Identity)
		if !ok {
			return nil, fmt.Errorf("backup encryption key %q is not an X25519 identity", path)
		}
		return x, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read backup encryption key %q: %w", path, err)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("generate backup encryption key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	body := "# backup encryption key, keep a copy off this machine\n# public key: " + id.Recipient().String() + "\n" + id.String() + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return nil, fmt.Errorf("write backup encryption key: %w", err)
	}
	return id, nil
}
