package ingress

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	snapshotMagic   = "LRIS"
	snapshotVersion = byte(1)
	snapshotAAD     = "ingress-snapshot"
	// SnapshotKeyPurpose is the secrets.Manager.DeriveKey purpose for the snapshot seal.
	SnapshotKeyPurpose = "ingress-snapshot"
	// SnapshotFilename is the snapshot's name inside the data directory.
	SnapshotFilename = "ingress-snapshot.bin"
)

var (
	// ErrNoSnapshot means no snapshot file exists yet.
	ErrNoSnapshot = errors.New("ingress: no route snapshot")
	// ErrSnapshotCorrupt means the file is truncated, tampered with or sealed with another key.
	ErrSnapshotCorrupt = errors.New("ingress: route snapshot is corrupt")
	// ErrSnapshotStale means the snapshot was taken with a different listener setup.
	ErrSnapshotStale = errors.New("ingress: route snapshot was taken with different listeners")
)

// SnapshotData is the last applied Caddy config plus what Driver needs to resume.
type SnapshotData struct {
	SavedAt     time.Time       `json:"saved_at"`
	Fingerprint string          `json:"fingerprint"`
	Ports       []int           `json:"ports"`
	Config      json.RawMessage `json:"config"`
}

// SnapshotStore keeps the last applied ingress config on disk, sealed with a
// key derived from the master key (the config carries DNS tokens, private keys
// and basic auth hashes), so a restarted process can serve before it reconciles.
type SnapshotStore struct {
	path string
	aead cipher.AEAD
}

// NewSnapshotStore seals snapshots at path with a 32 byte key.
func NewSnapshotStore(path string, key []byte) (*SnapshotStore, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("ingress: snapshot key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("ingress: snapshot cipher: %w", err)
	}
	return &SnapshotStore{path: path, aead: aead}, nil
}

// Path is the snapshot file location.
func (s *SnapshotStore) Path() string { return s.path }

// Save writes data atomically: a crash mid-write leaves the previous snapshot.
func (s *SnapshotStore) Save(data SnapshotData) error {
	plain, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("ingress: snapshot marshal: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("ingress: snapshot nonce: %w", err)
	}
	out := append([]byte(snapshotMagic), snapshotVersion)
	out = append(out, nonce...)
	out = s.aead.Seal(out, nonce, plain, []byte(snapshotAAD))

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".ingress-snapshot-*")
	if err != nil {
		return fmt.Errorf("ingress: snapshot temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingress: snapshot write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingress: snapshot sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ingress: snapshot close: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("ingress: snapshot rename: %w", err)
	}
	return nil
}

// Load reads and opens the snapshot. wantFingerprint must match the one saved.
func (s *SnapshotStore) Load(wantFingerprint string) (SnapshotData, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return SnapshotData{}, ErrNoSnapshot
	}
	if err != nil {
		return SnapshotData{}, fmt.Errorf("ingress: snapshot read: %w", err)
	}
	head := len(snapshotMagic) + 1
	ns := s.aead.NonceSize()
	if len(raw) < head+ns+s.aead.Overhead() || string(raw[:len(snapshotMagic)]) != snapshotMagic || raw[len(snapshotMagic)] != snapshotVersion {
		return SnapshotData{}, ErrSnapshotCorrupt
	}
	plain, err := s.aead.Open(nil, raw[head:head+ns], raw[head+ns:], []byte(snapshotAAD))
	if err != nil {
		return SnapshotData{}, ErrSnapshotCorrupt
	}
	var data SnapshotData
	if err := json.Unmarshal(plain, &data); err != nil || len(data.Config) == 0 {
		return SnapshotData{}, ErrSnapshotCorrupt
	}
	if data.Fingerprint != wantFingerprint {
		return SnapshotData{}, ErrSnapshotStale
	}
	return data, nil
}

// Fingerprint identifies the listener setup a config was built for, so a
// snapshot is never replayed onto different sockets.
func (s *InheritedSockets) Fingerprint() string {
	if !s.Active() {
		return "bound"
	}
	return "fd:" + strconv.Itoa(s.HTTPS) + ":" + strconv.Itoa(s.HTTP)
}
