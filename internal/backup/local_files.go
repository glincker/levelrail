package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvLocalRoot names the one directory local backup targets may live under.
const EnvLocalRoot = "APP_BACKUP_LOCAL_ROOT"

// LocalEndpointScheme marks a destination that is a directory on the control plane host.
const LocalEndpointScheme = store.LocalEndpointScheme

// ErrLocalRootUnset means a local target was used but no confinement root is configured.
var ErrLocalRootUnset = errors.New("backup: local targets need " + EnvLocalRoot + " set to an absolute directory")

// ErrLocalPathEscapes means a local target path resolves outside the configured root.
var ErrLocalPathEscapes = errors.New("backup: local path leaves " + EnvLocalRoot)

// LocalRootFromEnv returns the confinement root, or "" when unset or not absolute.
func LocalRootFromEnv(lookup func(string) (string, bool)) string {
	v, ok := lookup(EnvLocalRoot)
	v = strings.TrimSpace(v)
	if !ok || !filepath.IsAbs(v) {
		return ""
	}
	return filepath.Clean(v)
}

// IsLocal reports whether d is a host directory destination.
func (d Destination) IsLocal() bool { return strings.HasPrefix(d.Endpoint, LocalEndpointScheme) }

// CleanLocalPath validates a user supplied sub path: relative, no traversal.
func CleanLocalPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	c := filepath.Clean(p)
	if p == "" || c == "." || filepath.IsAbs(c) || c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
		return "", errors.New("local path must be a relative directory inside the backup root")
	}
	return c, nil
}

// localPath joins root, the target directory and key, refusing anything that
// leaves root, including through symlinks already on disk.
func (r Routed) localPath(d Destination, key string) (string, error) {
	if r.LocalRoot == "" {
		return "", ErrLocalRootUnset
	}
	sub, err := CleanLocalPath(d.Bucket)
	if err != nil {
		return "", err
	}
	full := filepath.Join(r.LocalRoot, sub, filepath.FromSlash(key))
	if !within(r.LocalRoot, full) {
		return "", ErrLocalPathEscapes
	}
	if resolved, rerr := filepath.EvalSymlinks(deepestExisting(full)); rerr == nil {
		root, eerr := filepath.EvalSymlinks(r.LocalRoot)
		if eerr != nil || !within(root, resolved) {
			return "", ErrLocalPathEscapes
		}
	}
	return full, nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func deepestExisting(p string) string {
	for {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

func (r Routed) localUpload(dest Destination, key string, rd io.Reader) error {
	p, err := r.localPath(dest, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("backup: local upload: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return fmt.Errorf("backup: local upload: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, rd); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("backup: local upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("backup: local upload: %w", err)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return fmt.Errorf("backup: local upload: %w", err)
	}
	return nil
}

func (r Routed) localDownload(dest Destination, key string) (io.ReadCloser, error) {
	p, err := r.localPath(dest, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // p is confined to the local root by localPath
	if err != nil {
		return nil, fmt.Errorf("backup: local download: %w", err)
	}
	return f, nil
}

func (r Routed) localDelete(dest Destination, key string) error {
	p, err := r.localPath(dest, key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup: local delete: %w", err)
	}
	return nil
}

func (r Routed) localList(dest Destination, prefix string) ([]RemoteObject, error) {
	base, err := r.localPath(dest, "")
	if err != nil {
		return nil, err
	}
	var out []RemoteObject
	werr := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".upload-") {
			return nil
		}
		rel, rerr := filepath.Rel(base, p)
		if rerr != nil {
			return rerr
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, ierr := e.Info()
		if ierr != nil {
			return ierr
		}
		out = append(out, RemoteObject{Key: key, Size: info.Size()})
		return nil
	})
	if werr != nil {
		return nil, fmt.Errorf("backup: local list: %w", werr)
	}
	return out, nil
}

func (r Routed) localTest(dest Destination) error {
	const probe = ".probe-local"
	if err := r.localUpload(dest, probe, strings.NewReader("ok")); err != nil {
		return err
	}
	rc, err := r.localDownload(dest, probe)
	if err != nil {
		return err
	}
	b, rerr := io.ReadAll(rc)
	_ = rc.Close()
	if rerr != nil || string(b) != "ok" {
		return errors.New("backup: test local path: object read back did not match what was written")
	}
	return r.localDelete(dest, probe)
}
