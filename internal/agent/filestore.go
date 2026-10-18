package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// ErrFileStoreDisabled means the agent has no storage directory configured, so it refuses every file operation.
var ErrFileStoreDisabled = errors.New("agent: file storage is not enabled on this node (no storage directory configured)")

// ErrFilePathEscapes is returned for any path that would resolve outside the storage root.
var ErrFilePathEscapes = errors.New("agent: file path escapes the storage directory")

type fileRootKey struct{}

// WithFileRoot enables file operations confined to dir for sessions served under ctx.
func WithFileRoot(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, fileRootKey{}, dir)
}

func fileRootFrom(ctx context.Context) string {
	dir, _ := ctx.Value(fileRootKey{}).(string)
	return dir
}

// FileStore performs object operations strictly inside root.
type FileStore struct{ root string }

// NewFileStore returns a store rooted at dir, creating it when missing.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, ErrFileStoreDisabled
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve storage directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("agent: create storage directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve storage directory: %w", err)
	}
	return &FileStore{root: resolved}, nil
}

func (s *FileStore) inside(p string) bool {
	return p == s.root || strings.HasPrefix(p, s.root+string(filepath.Separator))
}

// resolve maps a slash-separated relative path to an absolute path under root, rejecting traversal, absolute paths and symlink escapes.
func (s *FileStore) resolve(rel string) (string, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, 0) {
		return "", ErrFilePathEscapes
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", ErrFilePathEscapes
		}
	}
	full := filepath.Join(s.root, filepath.FromSlash(rel))
	if !s.inside(full) {
		return "", ErrFilePathEscapes
	}
	dir := filepath.Dir(full)
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			if !s.inside(resolved) {
				return "", ErrFilePathEscapes
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) || dir == s.root {
			return "", fmt.Errorf("agent: resolve %q: %w", rel, err)
		}
		dir = filepath.Dir(dir)
	}
	return full, nil
}

// Put writes r to rel through a temp file so a failed upload never leaves a partial object visible.
func (s *FileStore) Put(rel string, r io.Reader, maxBytes int64) error {
	full, err := s.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("agent: create directory for %q: %w", rel, err)
	}
	if _, err := s.resolve(rel); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return fmt.Errorf("agent: create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	src := r
	if maxBytes > 0 {
		src = io.LimitReader(r, maxBytes+1)
	}
	n, err := io.Copy(tmp, src)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: write %q: %w", rel, err)
	}
	if maxBytes > 0 && n > maxBytes {
		_ = tmp.Close()
		return fmt.Errorf("agent: %q exceeds the %d byte limit", rel, maxBytes)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agent: close %q: %w", rel, err)
	}
	if err := os.Rename(tmp.Name(), full); err != nil {
		return fmt.Errorf("agent: commit %q: %w", rel, err)
	}
	return nil
}

// Open returns the object at rel for reading.
func (s *FileStore) Open(rel string) (io.ReadCloser, error) {
	full, err := s.resolve(rel)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full) //nolint:gosec // full is confined to the storage root by resolve
	if err != nil {
		return nil, fmt.Errorf("agent: open %q: %w", rel, err)
	}
	return f, nil
}

// Delete removes rel; a missing object is success.
func (s *FileStore) Delete(rel string) error {
	full, err := s.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("agent: delete %q: %w", rel, err)
	}
	return nil
}

// List returns "<size>\t<path>" lines for every file whose path starts with prefix.
func (s *FileStore) List(prefix string) (io.ReadCloser, error) {
	if prefix != "" {
		if _, err := s.resolve(prefix); err != nil {
			return nil, err
		}
	}
	var lines []string
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".upload-") {
			return nil
		}
		rel, rerr := filepath.Rel(s.root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, prefix) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		lines = append(lines, fmt.Sprintf("%d\t%s\n", info.Size(), rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("agent: list %q: %w", prefix, err)
	}
	sort.Strings(lines)
	return io.NopCloser(strings.NewReader(strings.Join(lines, ""))), nil
}

// attachFile serves one FileOp over the exec frame stream, mirroring attachArchive's copy-in and copy-out shapes.
func (r *ExecRelay) attachFile(ctx context.Context, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) (io.ReadCloser, error) {
	store, err := NewFileStore(fileRootFrom(ctx))
	if err != nil {
		return nil, err
	}
	op := req.GetFile()
	switch op.GetKind() {
	case agentpb.FileOp_KIND_PUT:
		pr, pw := io.Pipe()
		if !ex.attach(pr) {
			_ = pr.Close()
			return nil, nil
		}
		go func() { _ = pw.CloseWithError(store.Put(op.GetPath(), stdinR, op.GetMaxBytes())) }()
		return pr, nil
	case agentpb.FileOp_KIND_GET:
		return attachReader(ex, func() (io.ReadCloser, error) { return store.Open(op.GetPath()) })
	case agentpb.FileOp_KIND_LIST:
		return attachReader(ex, func() (io.ReadCloser, error) { return store.List(op.GetPath()) })
	case agentpb.FileOp_KIND_DELETE:
		if err := store.Delete(op.GetPath()); err != nil {
			return nil, err
		}
		return attachReader(ex, func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil })
	default:
		return nil, errors.New("agent: unsupported file operation")
	}
}

func attachReader(ex *agentExec, open func() (io.ReadCloser, error)) (io.ReadCloser, error) {
	rc, err := open()
	if err != nil {
		return nil, err
	}
	if !ex.attach(rc) {
		_ = rc.Close()
		return nil, nil
	}
	return rc, nil
}
