// Package archive moves cold rows out of the live database into zstd
// compressed NDJSON bundles in a local directory, and can list, browse,
// restore and verify them.
package archive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/klauspost/compress/zstd"
)

// DirName is the bundle directory's name under the data dir.
const DirName = "archives"

// Kind names a group of tables archived together.
type Kind string

// Archive kinds.
const (
	KindAudit     Kind = "audit"
	KindDeploys   Kind = "deploys"
	KindPipelines Kind = "pipelines"
)

// Kinds lists every archivable kind.
var Kinds = []Kind{KindAudit, KindDeploys, KindPipelines}

var (
	// ErrInvalidID means the id does not match the bundle name pattern.
	ErrInvalidID = errors.New("invalid archive id")
	// ErrNotFound means no bundle with that id exists.
	ErrNotFound = errors.New("archive not found")
	// ErrNothingToArchive means no row is old enough to leave the live tables.
	ErrNothingToArchive = errors.New("nothing to archive")
	// ErrChecksum means a bundle no longer matches its recorded checksum.
	ErrChecksum = errors.New("archive checksum mismatch")
)

const (
	bundleSuffix = ".ndjson.zst"
	metaSuffix   = ".json"
	nameLayout   = "20060102T150405Z"
	maxLineBytes = 8 << 20
)

var idPattern = regexp.MustCompile(`^(audit|deploys|pipelines)-\d{8}T\d{6}Z-[a-f0-9]{6}$`)

// Bundle describes one archive bundle.
type Bundle struct {
	ID         string         `json:"id"`
	Kind       Kind           `json:"kind"`
	CreatedAt  time.Time      `json:"created_at"`
	Cutoff     time.Time      `json:"cutoff"`
	Rows       map[string]int `json:"rows"`
	TotalRows  int            `json:"total_rows"`
	SizeBytes  int64          `json:"size_bytes"`
	SHA256     string         `json:"sha256"`
	VerifiedAt *time.Time     `json:"verified_at,omitempty"`
	VerifiedOK *bool          `json:"verified_ok,omitempty"`
	RestoredAt *time.Time     `json:"restored_at,omitempty"`
}

type line struct {
	Table string                     `json:"t"`
	Row   map[string]json.RawMessage `json:"r"`
}

func validID(id string) bool { return idPattern.MatchString(id) }

func (m *Manager) bundlePath(id string) string { return filepath.Join(m.dir, id+bundleSuffix) }
func (m *Manager) metaPath(id string) string   { return filepath.Join(m.dir, id+metaSuffix) }

func (m *Manager) readMeta(id string) (Bundle, error) {
	if !validID(id) {
		return Bundle{}, ErrInvalidID
	}
	raw, err := os.ReadFile(m.metaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return Bundle{}, ErrNotFound
	}
	if err != nil {
		return Bundle{}, fmt.Errorf("archive: read meta %s: %w", id, err)
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, fmt.Errorf("archive: decode meta %s: %w", id, err)
	}
	return b, nil
}

func (m *Manager) writeMeta(b Bundle) error {
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("archive: encode meta %s: %w", b.ID, err)
	}
	tmp := m.metaPath(b.ID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("archive: write meta %s: %w", b.ID, err)
	}
	if err := os.Rename(tmp, m.metaPath(b.ID)); err != nil {
		return fmt.Errorf("archive: commit meta %s: %w", b.ID, err)
	}
	return nil
}

type bundleWriter struct {
	f     *os.File
	zw    *zstd.Encoder
	enc   *json.Encoder
	hash  io.Writer
	sum   func() string
	rows  map[string]int
	total int
}

func newBundleWriter(path string) (*bundleWriter, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is built from a generated archive id
	if err != nil {
		return nil, fmt.Errorf("archive: create bundle: %w", err)
	}
	h := sha256.New()
	zw, err := zstd.NewWriter(io.MultiWriter(f, h))
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("archive: zstd writer: %w", err)
	}
	return &bundleWriter{f: f, zw: zw, enc: json.NewEncoder(zw), hash: h, sum: func() string { return hex.EncodeToString(h.Sum(nil)) }, rows: map[string]int{}}, nil
}

func (w *bundleWriter) add(table string, row map[string]any) error {
	raw := make(map[string]json.RawMessage, len(row))
	for k, v := range row {
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("archive: encode %s.%s: %w", table, k, err)
		}
		raw[k] = b
	}
	if err := w.enc.Encode(line{Table: table, Row: raw}); err != nil {
		return fmt.Errorf("archive: write %s row: %w", table, err)
	}
	w.rows[table]++
	w.total++
	return nil
}

func (w *bundleWriter) finish() (size int64, sum string, err error) {
	if err = w.zw.Close(); err != nil {
		_ = w.f.Close()
		return 0, "", fmt.Errorf("archive: finish zstd: %w", err)
	}
	if err = w.f.Sync(); err != nil {
		_ = w.f.Close()
		return 0, "", fmt.Errorf("archive: sync bundle: %w", err)
	}
	st, err := w.f.Stat()
	if err != nil {
		_ = w.f.Close()
		return 0, "", fmt.Errorf("archive: stat bundle: %w", err)
	}
	if err = w.f.Close(); err != nil {
		return 0, "", fmt.Errorf("archive: close bundle: %w", err)
	}
	return st.Size(), w.sum(), nil
}

// fileSHA256 hashes the compressed bundle as stored.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is built from a validated archive id
	if err != nil {
		return "", fmt.Errorf("archive: open bundle: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("archive: hash bundle: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// scanBundle streams every line to fn; fn returning false stops early.
func (m *Manager) scanBundle(id string, fn func(line) (bool, error)) error {
	f, err := os.Open(m.bundlePath(id)) //nolint:gosec // validated id
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive: open bundle %s: %w", id, err)
	}
	defer func() { _ = f.Close() }()
	zr, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return fmt.Errorf("archive: zstd reader %s: %w", id, err)
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return fmt.Errorf("archive: decode line in %s: %w", id, err)
		}
		more, err := fn(l)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("archive: read bundle %s: %w", id, err)
	}
	return nil
}
