package sourceupload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// ErrNotFound is returned when an upload is unknown or already expired.
var ErrNotFound = errors.New("sourceupload: upload not found")

// ErrInvalidName is returned for an app name or hash that is not a safe path segment.
var ErrInvalidName = errors.New("sourceupload: invalid app name or hash")

var (
	hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
)

// ValidHash reports whether s is a lowercase hex sha256.
func ValidHash(s string) bool { return hashPattern.MatchString(s) }

// Meta describes one stored upload.
type Meta struct {
	Hash      string    `json:"hash"`
	App       string    `json:"app"`
	Size      int64     `json:"size"`
	Files     int       `json:"file_count"`
	Expanded  int64     `json:"extracted_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// Store keeps archives as <root>/<app>/<hash>.tar.gz with a .json sidecar.
type Store struct {
	root   string
	limits Limits
	now    func() time.Time
}

// NewStore returns a Store rooted at dir. The directory is created on first Put.
func NewStore(dir string, lim Limits) *Store {
	return &Store{root: dir, limits: lim, now: time.Now}
}

func (s *Store) appDir(app string) (string, error) {
	if !namePattern.MatchString(app) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, app)
	}
	return filepath.Join(s.root, app), nil
}

// Put streams r into the store, enforcing maxBytes on the compressed size and
// validating the archive before it becomes visible. deduped is true when the
// same content already existed for the app.
func (s *Store) Put(app string, r io.Reader, maxBytes int64) (meta Meta, deduped bool, err error) {
	dir, err := s.appDir(app)
	if err != nil {
		return Meta{}, false, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: create %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".incoming-*")
	if err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	h := sha256.New()
	src := r
	if maxBytes > 0 {
		src = io.LimitReader(r, maxBytes+1)
	}
	n, err := io.Copy(io.MultiWriter(tmp, h), src)
	if err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: receive archive: %w", err)
	}
	if maxBytes > 0 && n > maxBytes {
		return Meta{}, false, fmt.Errorf("%w: archive is larger than %d bytes", ErrTooLarge, maxBytes)
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: rewind: %w", err)
	}
	stats, err := Inspect(tmp, s.limits)
	if err != nil {
		return Meta{}, false, err
	}
	meta = Meta{Hash: hash, App: app, Size: n, Files: stats.Files, Expanded: stats.Bytes, CreatedAt: s.now().UTC()}
	if existing, gerr := s.Get(app, hash); gerr == nil {
		now := s.now()
		_ = os.Chtimes(filepath.Join(dir, hash+".tar.gz"), now, now)
		return existing, true, nil
	}
	if err := tmp.Close(); err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: close temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, hash+".tar.gz")); err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: store archive: %w", err)
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return Meta{}, false, fmt.Errorf("sourceupload: encode meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, hash+".json"), b, 0o600); err != nil {
		_ = os.Remove(filepath.Join(dir, hash+".tar.gz"))
		return Meta{}, false, fmt.Errorf("sourceupload: store meta: %w", err)
	}
	return meta, false, nil
}

// Get returns the metadata of a stored upload, or ErrNotFound.
func (s *Store) Get(app, hash string) (Meta, error) {
	dir, err := s.appDir(app)
	if err != nil || !ValidHash(hash) {
		return Meta{}, fmt.Errorf("%w: %q %q", ErrInvalidName, app, hash)
	}
	b, err := os.ReadFile(filepath.Join(dir, hash+".json")) //nolint:gosec // app and hash validated above
	if errors.Is(err, os.ErrNotExist) {
		return Meta{}, ErrNotFound
	}
	if err != nil {
		return Meta{}, fmt.Errorf("sourceupload: read meta: %w", err)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, fmt.Errorf("sourceupload: decode meta: %w", err)
	}
	return m, nil
}

// Open returns the stored archive for reading.
func (s *Store) Open(app, hash string) (*os.File, error) {
	if _, err := s.Get(app, hash); err != nil {
		return nil, err
	}
	dir, _ := s.appDir(app)
	f, err := os.Open(filepath.Join(dir, hash+".tar.gz")) //nolint:gosec // app and hash validated by Get
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sourceupload: open archive: %w", err)
	}
	return f, nil
}

// Extract unpacks a stored upload under dest.
func (s *Store) Extract(app, hash, dest string) (Stats, error) {
	f, err := s.Open(app, hash)
	if err != nil {
		return Stats{}, err
	}
	defer func() { _ = f.Close() }()
	return Extract(f, dest, s.limits)
}

// List returns an app's stored uploads, newest first.
func (s *Store) List(app string) ([]Meta, error) {
	dir, err := s.appDir(app)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sourceupload: list %q: %w", dir, err)
	}
	var out []Meta
	for _, e := range entries {
		name := e.Name()
		if len(name) != 64+len(".json") || filepath.Ext(name) != ".json" {
			continue
		}
		if m, err := s.Get(app, name[:64]); err == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Usage returns the total bytes of stored archives.
func (s *Store) Usage() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() && filepath.Ext(d.Name()) == ".gz" {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// Cleanup removes uploads last stored or re-sent more than retention ago, plus
// abandoned partial files, and returns how many uploads it removed.
func (s *Store) Cleanup(retention time.Duration) (int, error) {
	apps, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sourceupload: list %q: %w", s.root, err)
	}
	cutoff := s.now().Add(-retention)
	removed := 0
	for _, a := range apps {
		if !a.IsDir() || !namePattern.MatchString(a.Name()) {
			continue
		}
		dir := filepath.Join(s.root, a.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			info, err := f.Info()
			if err != nil || !info.ModTime().Before(cutoff) {
				continue
			}
			name := f.Name()
			switch {
			case filepath.Ext(name) == ".gz" && len(name) == 64+len(".tar.gz"):
				_ = os.Remove(filepath.Join(dir, name[:64]+".json"))
				if os.Remove(filepath.Join(dir, name)) == nil {
					removed++
				}
			case len(name) > 10 && name[:10] == ".incoming-":
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}
	return removed, nil
}
