// Package projectarchive reads and writes the .lrx project archive: a
// gzip tar holding declarative spec files, optional encrypted secrets and
// optional data dumps, sealed by a manifest of checksums.
package projectarchive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Format identifiers written into every manifest.
const (
	FormatVersion = 1
	FormatKind    = "levelrail.project-archive"
	ManifestName  = "manifest.json"
	SecretsName   = "secrets.enc"
	SpecPrefix    = "spec/"
	DataPrefix    = "data/"
	// EnvMaxSpecBytes names the env var that caps the in-memory size of
	// spec and secret entries read from an archive.
	EnvMaxSpecBytes     = "APP_PROJECT_ARCHIVE_MAX_SPEC_BYTES"
	defaultMaxSpecBytes = 32 << 20
)

// Errors a caller can map to a response.
var (
	ErrChecksum = errors.New("projectarchive: checksum mismatch, the archive was changed or damaged")
	ErrFormat   = errors.New("projectarchive: not a project archive or an unsupported format version")
)

// FileEntry is one archived file as the manifest records it.
type FileEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest is the last entry of the archive and covers every other entry.
type Manifest struct {
	Format          int         `json:"format"`
	Kind            string      `json:"kind"`
	SourceVersion   string      `json:"source_version"`
	Project         string      `json:"project"`
	CreatedAt       string      `json:"created_at"`
	Apps            []string    `json:"apps,omitempty"`
	SecretsIncluded bool        `json:"secrets_included"`
	DataIncluded    bool        `json:"data_included"`
	Warnings        []string    `json:"warnings,omitempty"`
	Files           []FileEntry `json:"files"`
}

// Writer streams entries into a gzip tar and finishes with the manifest.
type Writer struct {
	gz    *gzip.Writer
	tw    *tar.Writer
	files []FileEntry
}

// NewWriter starts an archive on w.
func NewWriter(w io.Writer) *Writer {
	gz := gzip.NewWriter(w)
	return &Writer{gz: gz, tw: tar.NewWriter(gz)}
}

// AddBytes writes one small entry.
func (w *Writer) AddBytes(name string, b []byte) error {
	if err := validName(name); err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if err := w.tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), Format: tar.FormatPAX}); err != nil {
		return err
	}
	if _, err := w.tw.Write(b); err != nil {
		return err
	}
	w.files = append(w.files, FileEntry{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(b))})
	return nil
}

// AddStream spools r to a temp file first because a tar header needs the
// size, then copies it in, so a large dump never sits in memory.
func (w *Writer) AddStream(name string, r io.Reader) error {
	if err := validName(name); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "lrx-data-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()); _ = tmp.Close() }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := w.tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: n, Format: tar.FormatPAX}); err != nil {
		return err
	}
	if _, err := io.Copy(w.tw, tmp); err != nil {
		return err
	}
	w.files = append(w.files, FileEntry{Name: name, SHA256: hex.EncodeToString(h.Sum(nil)), Size: n})
	return nil
}

// Close writes the manifest last and flushes the archive.
func (w *Writer) Close(m Manifest) error {
	m.Format, m.Kind = FormatVersion, FormatKind
	sort.Slice(w.files, func(i, j int) bool { return w.files[i].Name < w.files[j].Name })
	m.Files = w.files
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := w.tw.WriteHeader(&tar.Header{Name: ManifestName, Mode: 0o600, Size: int64(len(raw)), Format: tar.FormatPAX}); err != nil {
		return err
	}
	if _, err := w.tw.Write(raw); err != nil {
		return err
	}
	if err := w.tw.Close(); err != nil {
		return err
	}
	return w.gz.Close()
}

// Archive is a verified archive. Data entries are checksummed while
// reading and not kept, a restore streams them from the original file.
type Archive struct {
	Manifest Manifest
	// Spec maps each spec file name, without the prefix, to its bytes.
	Spec map[string][]byte
	// Secrets is the sealed secrets blob, empty when none were included.
	Secrets []byte
}

func maxSpecBytes() int64 {
	if v, err := strconv.ParseInt(os.Getenv(EnvMaxSpecBytes), 10, 64); err == nil && v > 0 {
		return v
	}
	return defaultMaxSpecBytes
}

// Read parses r and verifies the manifest and every checksum. Any extra,
// missing or altered entry is an error.
func Read(r io.Reader) (*Archive, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, ErrFormat
	}
	tr := tar.NewReader(gz)
	seen := map[string]FileEntry{}
	mem := map[string][]byte{}
	var manifestRaw []byte
	budget := maxSpecBytes()
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ErrFormat
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%w: unexpected entry type for %q", ErrFormat, h.Name)
		}
		if err := validName(h.Name); err != nil && h.Name != ManifestName {
			return nil, err
		}
		if _, dup := seen[h.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate entry %q", ErrFormat, h.Name)
		}
		hash := sha256.New()
		keep := h.Name == ManifestName || !strings.HasPrefix(h.Name, DataPrefix)
		var buf []byte
		var n int64
		if keep {
			budget -= h.Size
			if budget < 0 {
				return nil, fmt.Errorf("%w: spec entries exceed %s", ErrFormat, EnvMaxSpecBytes)
			}
			buf, err = io.ReadAll(io.TeeReader(io.LimitReader(tr, h.Size), hash))
			n = int64(len(buf))
		} else {
			n, err = io.Copy(hash, tr) //nolint:gosec // hashed and discarded, never held in memory
		}
		if err != nil {
			return nil, ErrFormat
		}
		if h.Name == ManifestName {
			manifestRaw = buf
			continue
		}
		seen[h.Name] = FileEntry{Name: h.Name, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: n}
		if keep {
			mem[h.Name] = buf
		}
	}
	// The tar reader stops at its end marker, so drain to reach the gzip CRC check.
	if _, err := io.Copy(io.Discard, gz); err != nil { //nolint:gosec // only the gzip trailer remains after the tar end marker
		return nil, ErrChecksum
	}
	if manifestRaw == nil {
		return nil, fmt.Errorf("%w: no manifest", ErrFormat)
	}
	var m Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil || m.Kind != FormatKind || m.Format != FormatVersion {
		return nil, ErrFormat
	}
	if len(m.Files) != len(seen) {
		return nil, ErrChecksum
	}
	for _, f := range m.Files {
		got, ok := seen[f.Name]
		if !ok || got.SHA256 != f.SHA256 || got.Size != f.Size {
			return nil, ErrChecksum
		}
	}
	a := &Archive{Manifest: m, Spec: map[string][]byte{}, Secrets: mem[SecretsName]}
	for name, b := range mem {
		if strings.HasPrefix(name, SpecPrefix) {
			a.Spec[strings.TrimPrefix(name, SpecPrefix)] = b
		}
	}
	return a, nil
}

func validName(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
		return fmt.Errorf("%w: unsafe entry name %q", ErrFormat, name)
	}
	return nil
}
