package proxyroutes

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxManagedFileSize bounds how much of a candidate file is read.
const maxManagedFileSize = 64 << 10

// fileMode is world-readable so a proxy running as another user can load it.
const fileMode os.FileMode = 0o644

// ErrForeignFile is returned when a file with a managed name was not
// written by this package (no header, not a regular file).
var ErrForeignFile = errors.New("file was not written by this instance")

// Dir is a proxy configuration directory, resolved once to its real path
// and re-checked before every change.
type Dir struct {
	ns   Namespace
	path string
	real string
}

// OpenDir resolves path. It must be absolute and an existing directory. If
// the last component is a symlink it must resolve inside the same parent.
func OpenDir(ns Namespace, path string) (*Dir, error) {
	if ns == "" {
		return nil, errors.New("proxyroutes: empty namespace")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("directory %q must be an absolute path", path)
	}
	clean := filepath.Clean(path)
	if clean == string(filepath.Separator) {
		return nil, errors.New("refusing to manage files in the root directory")
	}
	d := &Dir{ns: ns, path: clean}
	resolved, err := d.resolve()
	if err != nil {
		return nil, err
	}
	d.real = resolved
	return d, nil
}

func (d *Dir) resolve() (string, error) {
	resolved, err := filepath.EvalSymlinks(d.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("directory %s does not exist on this host", d.path)
		}
		return "", fmt.Errorf("resolve directory %s: %w", d.path, err)
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat directory %s: %w", d.path, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", d.path)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(d.path))
	if err != nil {
		return "", fmt.Errorf("resolve parent of %s: %w", d.path, err)
	}
	if !within(resolved, parent) {
		return "", fmt.Errorf("directory %s is a symlink to %s, outside %s", d.path, resolved, filepath.Dir(d.path))
	}
	return resolved, nil
}

func within(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Path is the directory as configured.
func (d *Dir) Path() string { return d.path }

// recheck guards against the directory being swapped for a symlink after
// OpenDir resolved it.
func (d *Dir) recheck() error {
	resolved, err := d.resolve()
	if err != nil {
		return err
	}
	if resolved != d.real {
		return fmt.Errorf("directory %s now resolves to %s, refusing to continue", d.path, resolved)
	}
	return nil
}

// target joins a managed file name onto the real directory, refusing any
// name that is not a bare managed file name.
func (d *Dir) target(name string) (string, error) {
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || !d.isManagedName(name) {
		return "", fmt.Errorf("%q is not a managed file name", name)
	}
	p := filepath.Join(d.real, name)
	if filepath.Dir(p) != d.real {
		return "", fmt.Errorf("%q escapes the directory", name)
	}
	return p, nil
}

func (d *Dir) isManagedName(name string) bool {
	return strings.HasPrefix(name, d.ns.FilePrefix()) && strings.HasSuffix(name, ".yaml") && len(name) > len(d.ns.FilePrefix())+len(".yaml")
}

// DomainOf is the domain a managed file name belongs to.
func (d *Dir) DomainOf(name string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, d.ns.FilePrefix()), ".yaml")
}

// read returns the content of a managed file, or ErrForeignFile when it is
// not a regular file carrying the header. Symlinks are never followed.
func (d *Dir) read(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrForeignFile
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // path is a validated managed name inside the checked directory
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, maxManagedFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	first, _, _ := bufio.NewReader(bytes.NewReader(body)).ReadLine()
	if len(body) > maxManagedFileSize || string(first) != d.ns.Header() {
		return nil, ErrForeignFile
	}
	return body, nil
}

// Managed lists the managed files currently in the directory by name, and
// separately the names that carry the prefix but are not ours.
func (d *Dir) Managed() (managed map[string][]byte, foreign []string, err error) {
	if err := d.recheck(); err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(d.real)
	if err != nil {
		return nil, nil, fmt.Errorf("list %s: %w", d.path, err)
	}
	managed = map[string][]byte{}
	for _, e := range entries {
		if !d.isManagedName(e.Name()) {
			continue
		}
		body, err := d.read(filepath.Join(d.real, e.Name()))
		switch {
		case errors.Is(err, ErrForeignFile):
			foreign = append(foreign, e.Name())
		case err != nil:
			return nil, nil, err
		default:
			managed[e.Name()] = body
		}
	}
	return managed, foreign, nil
}

// Write replaces name with content atomically (temp file plus rename in the
// same directory). An existing file with that name must be ours.
func (d *Dir) Write(name string, content []byte) error {
	if !bytes.HasPrefix(content, []byte(d.ns.Header()+"\n")) {
		return errors.New("refusing to write a file without the managed header")
	}
	if err := d.recheck(); err != nil {
		return err
	}
	path, err := d.target(name)
	if err != nil {
		return err
	}
	if _, err := d.read(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", name, err)
	}
	tmp, err := os.CreateTemp(d.real, "."+d.ns.FilePrefix()+"*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", d.path, err)
	}
	done := false
	defer func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Chmod(fileMode); err != nil {
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		done = true
		return fmt.Errorf("rename into %s: %w", name, err)
	}
	done = true
	return nil
}

// Remove deletes name only when it is a regular file carrying the header.
func (d *Dir) Remove(name string) error {
	if err := d.recheck(); err != nil {
		return err
	}
	path, err := d.target(name)
	if err != nil {
		return err
	}
	if _, err := d.read(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}
