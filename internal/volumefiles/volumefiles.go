// Package volumefiles browses and edits single files inside a named volume's
// host directory, confined to that directory by os.Root so traversal and
// symlink escapes fail in the kernel-facing layer rather than by string checks.
package volumefiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	envMaxUploadMB = "APP_VOLUME_FILE_MAX_UPLOAD_MB"
	envListLimit   = "APP_VOLUME_FILE_LIST_LIMIT"

	defaultMaxUploadMB = 64
	defaultListLimit   = 1000
)

var (
	// ErrPathEscapes is returned for any path that leaves the volume root.
	ErrPathEscapes = errors.New("volumefiles: path escapes the volume")
	// ErrNotFound means the path does not exist inside the volume.
	ErrNotFound = errors.New("volumefiles: no such file")
	// ErrIsDirectory means a file operation was aimed at a directory.
	ErrIsDirectory = errors.New("volumefiles: path is a directory")
	// ErrNotRegular means the path is a device, socket, fifo or similar.
	ErrNotRegular = errors.New("volumefiles: path is not a regular file")
	// ErrTooLarge means an upload exceeded the configured cap.
	ErrTooLarge = errors.New("volumefiles: upload exceeds the size limit")
)

// MaxUploadBytes is the per-file upload cap from APP_VOLUME_FILE_MAX_UPLOAD_MB.
func MaxUploadBytes() int64 {
	return int64(envPositive(envMaxUploadMB, defaultMaxUploadMB)) << 20
}

// ListLimit is the per-directory entry cap from APP_VOLUME_FILE_LIST_LIMIT.
func ListLimit() int {
	return envPositive(envListLimit, defaultListLimit)
}

func envPositive(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// EffectiveMax returns the smaller positive of requested and the env cap.
func EffectiveMax(requested int64) int64 {
	env := MaxUploadBytes()
	if requested > 0 && requested < env {
		return requested
	}
	return env
}

// EffectiveLimit returns the smaller positive of requested and the env cap.
func EffectiveLimit(requested int) int {
	env := ListLimit()
	if requested > 0 && requested < env {
		return requested
	}
	return env
}

// Entry is one item in a directory listing.
type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   string `json:"mod_time"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Store performs file operations strictly inside one volume directory.
type Store struct{ root *os.Root }

// Open confines operations to dir, normally a volume's host mountpoint.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("volumefiles: volume directory is empty")
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("volumefiles: open volume: %w", err)
	}
	return &Store{root: r}, nil
}

// Close releases the root handle.
func (s *Store) Close() error { return s.root.Close() }

// Clean maps a user path to a slash-separated path relative to the volume
// root, rejecting NUL bytes and any ".." component. "" and "/" mean the root.
func Clean(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", ErrPathEscapes
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", ErrPathEscapes
		}
	}
	c := path.Clean("/" + p)
	if c == "/" {
		return ".", nil
	}
	return strings.TrimPrefix(c, "/"), nil
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return ErrNotFound
	case strings.Contains(err.Error(), "path escapes from parent"):
		return ErrPathEscapes
	}
	return err
}

func typeOf(m fs.FileMode) string {
	switch {
	case m.IsDir():
		return "dir"
	case m&fs.ModeSymlink != 0:
		return "symlink"
	case m.IsRegular():
		return "file"
	}
	return "other"
}

// List returns up to limit entries of the directory at p, directories first,
// and whether more existed. Symlinks are listed, never followed.
func (s *Store) List(p string, limit int) ([]Entry, bool, error) {
	rel, err := Clean(p)
	if err != nil {
		return nil, false, err
	}
	d, err := s.root.Open(rel)
	if err != nil {
		return nil, false, mapErr(err)
	}
	defer func() { _ = d.Close() }()
	st, err := d.Stat()
	if err != nil {
		return nil, false, mapErr(err)
	}
	if !st.IsDir() {
		return nil, false, fmt.Errorf("volumefiles: %q is not a directory", p)
	}
	names, err := d.ReadDir(-1)
	if err != nil {
		return nil, false, fmt.Errorf("volumefiles: read directory: %w", err)
	}
	sort.Slice(names, func(i, j int) bool {
		di, dj := names[i].IsDir(), names[j].IsDir()
		if di != dj {
			return di
		}
		return names[i].Name() < names[j].Name()
	})
	truncated := limit > 0 && len(names) > limit
	if truncated {
		names = names[:limit]
	}
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		info, ierr := n.Info()
		if ierr != nil {
			continue
		}
		child := n.Name()
		if rel != "." {
			child = rel + "/" + n.Name()
		}
		out = append(out, Entry{
			Name: n.Name(), Path: child, Type: typeOf(info.Mode()), Size: info.Size(),
			Mode: info.Mode().Perm().String(), ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return out, truncated, nil
}

// Read opens the regular file at p. The caller closes it.
func (s *Store) Read(p string) (io.ReadCloser, int64, error) {
	rel, err := Clean(p)
	if err != nil {
		return nil, 0, err
	}
	f, err := s.root.Open(rel)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, mapErr(err)
	}
	if st.IsDir() {
		_ = f.Close()
		return nil, 0, ErrIsDirectory
	}
	if !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, ErrNotRegular
	}
	return f, st.Size(), nil
}

// Write stores r at p through a temp file and a rename, so a failed or
// oversized upload never leaves a partial file. Existing ownership and mode
// are kept so the app keeps access to what it owns.
func (s *Store) Write(p string, r io.Reader, maxBytes int64) (int64, error) {
	return s.WriteMode(p, r, maxBytes, 0)
}

// WriteMode is Write with a requested permission for the file. A zero mode keeps
// Write's behaviour; otherwise the result is never wider than mode, nor than the
// file it replaces.
func (s *Store) WriteMode(p string, r io.Reader, maxBytes int64, want fs.FileMode) (int64, error) {
	rel, err := Clean(p)
	if err != nil {
		return 0, err
	}
	if rel == "." {
		return 0, ErrIsDirectory
	}
	dir := path.Dir(rel)
	if dir != "." {
		if err := s.mkdirAllOwned(dir); err != nil {
			return 0, mapErr(err)
		}
	}
	mode := fs.FileMode(0o644)
	prev, perr := s.root.Lstat(rel)
	switch {
	case perr == nil && prev.IsDir():
		return 0, ErrIsDirectory
	case perr == nil && prev.Mode().IsRegular():
		mode = prev.Mode().Perm()
	case perr != nil && !errors.Is(perr, fs.ErrNotExist):
		return 0, mapErr(perr)
	}
	if want != 0 {
		if perr == nil {
			mode &= want.Perm()
		} else {
			mode = want.Perm()
		}
	}
	tmpName := path.Join(dir, ".upload-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	tmp, err := s.root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return 0, mapErr(err)
	}
	defer func() { _ = s.root.Remove(tmpName) }()
	src := r
	if maxBytes > 0 {
		src = io.LimitReader(r, maxBytes+1)
	}
	n, err := io.Copy(tmp, src)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, fmt.Errorf("volumefiles: write: %w", err)
	}
	if maxBytes > 0 && n > maxBytes {
		return 0, ErrTooLarge
	}
	ref := dir
	if perr == nil {
		ref = rel
	}
	if err := s.matchOwner(ref, tmpName); err != nil {
		return 0, err
	}
	if err := s.root.Chmod(tmpName, mode); err != nil {
		return 0, mapErr(err)
	}
	if err := s.root.Rename(tmpName, rel); err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

// Remove deletes exactly one non-directory entry; a symlink is removed, not followed.
func (s *Store) Remove(p string) error {
	rel, err := Clean(p)
	if err != nil {
		return err
	}
	if rel == "." {
		return ErrIsDirectory
	}
	st, err := s.root.Lstat(rel)
	if err != nil {
		return mapErr(err)
	}
	if st.IsDir() {
		return ErrIsDirectory
	}
	return mapErr(s.root.Remove(rel))
}

// EncodeEntries writes entries as JSON lines, plus a final truncated marker.
func EncodeEntries(w io.Writer, entries []Entry, truncated bool) error {
	enc := json.NewEncoder(w)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	if truncated {
		return enc.Encode(Entry{Truncated: true})
	}
	return nil
}

// DecodeEntries is the inverse of EncodeEntries.
func DecodeEntries(r io.Reader) ([]Entry, bool, error) {
	var out []Entry
	truncated := false
	dec := json.NewDecoder(r)
	for {
		var e Entry
		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				return out, truncated, nil
			}
			return nil, false, fmt.Errorf("volumefiles: decode listing: %w", err)
		}
		if e.Truncated {
			truncated = true
			continue
		}
		out = append(out, e)
	}
}

// Classify recovers a sentinel from an error that crossed the agent wire as text.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range []error{ErrPathEscapes, ErrNotFound, ErrIsDirectory, ErrNotRegular, ErrTooLarge} {
		if errors.Is(err, sentinel) || strings.Contains(err.Error(), sentinel.Error()) {
			return sentinel
		}
	}
	return err
}

// mkdirAllOwned creates dir and any missing parents, each owned like its parent where chown is allowed.
func (s *Store) mkdirAllOwned(dir string) error {
	parts := strings.Split(dir, "/")
	cur := ""
	for _, part := range parts {
		parent := cur
		if parent == "" {
			parent = "."
		}
		cur = path.Join(cur, part)
		if _, err := s.root.Lstat(cur); err == nil {
			continue
		}
		if err := s.root.Mkdir(cur, 0o755); err != nil {
			return err
		}
		if err := s.matchOwner(parent, cur); err != nil {
			return err
		}
	}
	return nil
}
