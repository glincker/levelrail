package preview

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const fileExt = ".jpg"

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// FileStore keeps thumbnails at <root>/<app>/<deployment>.jpg.
type FileStore struct{ root string }

// NewFileStore returns a store rooted at root, which is created on first write.
func NewFileStore(root string) *FileStore { return &FileStore{root: root} }

// FileEntry is one thumbnail file found on disk.
type FileEntry struct {
	App          string
	DeploymentID string
	Size         int64
}

// Path returns the file path for a deployment, rejecting names that could
// escape the root.
func (f *FileStore) Path(app, deploymentID string) (string, error) {
	if !safeName.MatchString(app) || !safeName.MatchString(deploymentID) || strings.Contains(app, "..") || strings.Contains(deploymentID, "..") {
		return "", fmt.Errorf("preview: unsafe path component app=%q deployment=%q", app, deploymentID)
	}
	return filepath.Join(f.root, app, deploymentID+fileExt), nil
}

// Write stores data atomically.
func (f *FileStore) Write(app, deploymentID string, data []byte) error {
	p, err := f.Path(app, deploymentID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("preview: create dir for %q: %w", p, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return fmt.Errorf("preview: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("preview: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("preview: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("preview: publish %q: %w", p, err)
	}
	return nil
}

// Remove deletes one thumbnail; a missing file is not an error.
func (f *FileStore) Remove(app, deploymentID string) error {
	p, err := f.Path(app, deploymentID)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("preview: remove %q: %w", p, err)
	}
	return nil
}

// RemoveApp deletes every thumbnail of app.
func (f *FileStore) RemoveApp(app string) error {
	if !safeName.MatchString(app) || strings.Contains(app, "..") {
		return fmt.Errorf("preview: unsafe app name %q", app)
	}
	if err := os.RemoveAll(filepath.Join(f.root, app)); err != nil {
		return fmt.Errorf("preview: remove app dir %q: %w", app, err)
	}
	return nil
}

// Exists reports whether the thumbnail file is present.
func (f *FileStore) Exists(app, deploymentID string) bool {
	p, err := f.Path(app, deploymentID)
	if err != nil {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// List returns every thumbnail file on disk. Stray temp files are removed.
func (f *FileStore) List() ([]FileEntry, error) {
	apps, err := os.ReadDir(f.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("preview: read %q: %w", f.root, err)
	}
	var out []FileEntry
	for _, a := range apps {
		if !a.IsDir() {
			continue
		}
		dir := filepath.Join(f.root, a.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("preview: read %q: %w", dir, err)
		}
		for _, e := range files {
			name := e.Name()
			if strings.HasPrefix(name, ".tmp-") {
				_ = os.Remove(filepath.Join(dir, name))
				continue
			}
			id, ok := strings.CutSuffix(name, fileExt)
			info, ierr := e.Info()
			if !ok || ierr != nil || !info.Mode().IsRegular() {
				continue
			}
			out = append(out, FileEntry{App: a.Name(), DeploymentID: id, Size: info.Size()})
		}
	}
	return out, nil
}

// RemoveEntry deletes a file found by List, plus its app directory if empty.
func (f *FileStore) RemoveEntry(e FileEntry) error {
	p := filepath.Join(f.root, e.App, e.DeploymentID+fileExt)
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("preview: remove %q: %w", p, err)
	}
	_ = os.Remove(filepath.Dir(p)) // only succeeds when empty
	return nil
}
