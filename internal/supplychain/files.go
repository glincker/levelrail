package supplychain

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const fileExt = ".sbom.json"

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// FileStore keeps SBOM documents at <root>/<app>/<attempt>.sbom.json.
type FileStore struct{ root string }

// NewFileStore returns a store rooted at root, which is created on first write.
func NewFileStore(root string) *FileStore { return &FileStore{root: root} }

// FileEntry is one SBOM file found on disk.
type FileEntry struct {
	App       string
	AttemptID string
	Size      int64
	ModTime   time.Time
}

// Path returns the file path for an attempt, rejecting names that could
// escape the root.
func (f *FileStore) Path(app, attemptID string) (string, error) {
	if !safeName.MatchString(app) || !safeName.MatchString(attemptID) || strings.Contains(app, "..") || strings.Contains(attemptID, "..") {
		return "", fmt.Errorf("supplychain: unsafe path component app=%q attempt=%q", app, attemptID)
	}
	return filepath.Join(f.root, app, attemptID+fileExt), nil
}

// Write stores data atomically.
func (f *FileStore) Write(app, attemptID string, data []byte) error {
	p, err := f.Path(app, attemptID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("supplychain: create dir for %q: %w", p, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return fmt.Errorf("supplychain: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("supplychain: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("supplychain: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("supplychain: publish %q: %w", p, err)
	}
	return nil
}

// Read returns the stored document, or ErrNotFound.
func (f *FileStore) Read(app, attemptID string) ([]byte, error) {
	p, err := f.Path(app, attemptID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // p comes from Path, which rejects unsafe components
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("supplychain: read %q: %w", p, err)
	}
	return data, nil
}

// Remove deletes one document; a missing file is not an error.
func (f *FileStore) Remove(app, attemptID string) error {
	p, err := f.Path(app, attemptID)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("supplychain: remove %q: %w", p, err)
	}
	return nil
}

// RemoveApp deletes every document of an app.
func (f *FileStore) RemoveApp(app string) error {
	if !safeName.MatchString(app) || strings.Contains(app, "..") {
		return fmt.Errorf("supplychain: unsafe app name %q", app)
	}
	if err := os.RemoveAll(filepath.Join(f.root, app)); err != nil {
		return fmt.Errorf("supplychain: remove files of %q: %w", app, err)
	}
	return nil
}

// List returns every SBOM file on disk.
func (f *FileStore) List() ([]FileEntry, error) {
	apps, err := os.ReadDir(f.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("supplychain: list %q: %w", f.root, err)
	}
	var out []FileEntry
	for _, a := range apps {
		if !a.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(f.root, a.Name()))
		if err != nil {
			return nil, fmt.Errorf("supplychain: list %q: %w", a.Name(), err)
		}
		for _, e := range files {
			id, ok := strings.CutSuffix(e.Name(), fileExt)
			if !ok || e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, FileEntry{App: a.Name(), AttemptID: id, Size: info.Size(), ModTime: info.ModTime()})
		}
	}
	return out, nil
}

// RemoveEntry deletes the file behind e.
func (f *FileStore) RemoveEntry(e FileEntry) error { return f.Remove(e.App, e.AttemptID) }
