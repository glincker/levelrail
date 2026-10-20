package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Artifact errors callers can match.
var (
	ErrArtifactTooLarge = errors.New("pipeline: artifact exceeds the size limit")
	ErrArtifactCorrupt  = errors.New("pipeline: artifact failed its sha256 check")
	ErrArtifactPath     = errors.New("pipeline: artifact path is outside the store")
)

// ArtifactLimits are the artifact size and retention caps, set from the
// APP_PIPELINE_ARTIFACT_* environment variables by the caller.
type ArtifactLimits struct {
	// MaxBytes caps one stored archive. MaxUnpackedBytes caps what it
	// expands to, so a small archive cannot fill the restore workspace.
	MaxBytes         int64
	MaxUnpackedBytes int64
	// MaxEntries caps the files, directories and links in one archive.
	MaxEntries int
	// MaxRunBytes caps all archives of one run together.
	MaxRunBytes int64
	Retention   time.Duration
}

// ArtifactIndex is the metadata persistence the artifact store needs.
// *store.DB satisfies it.
type ArtifactIndex interface {
	SavePipelineArtifact(ctx context.Context, a store.PipelineArtifact) error
	ListPipelineArtifacts(ctx context.Context, runID string) ([]store.PipelineArtifact, error)
	ListExpiredPipelineArtifacts(ctx context.Context, now time.Time) ([]store.PipelineArtifact, error)
	DeletePipelineArtifact(ctx context.Context, id string) error
}

// ArtifactStore keeps artifact archives on local disk under one directory,
// one subdirectory per run. File names derive from IDs, never from user input.
type ArtifactStore struct {
	dir string
	lim ArtifactLimits
	now func() time.Time
}

// NewArtifactStore creates dir if needed.
func NewArtifactStore(dir string, lim ArtifactLimits) (*ArtifactStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("pipeline: create artifact dir: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("pipeline: resolve artifact dir: %w", err)
	}
	return &ArtifactStore{dir: abs, lim: lim, now: time.Now}, nil
}

// Limits returns the configured caps.
func (s *ArtifactStore) Limits() ArtifactLimits { return s.lim }

// resolve maps a stored relative path to an absolute one, refusing anything
// that would leave the store directory.
func (s *ArtifactStore) resolve(rel string) (string, error) {
	full := filepath.Join(s.dir, filepath.FromSlash(rel))
	r, err := filepath.Rel(s.dir, full)
	if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrArtifactPath
	}
	return full, nil
}

// ArtifactWriter receives one archive's bytes, enforcing the size cap and
// hashing as it goes. Abort or Commit must be called exactly once.
type ArtifactWriter struct {
	s    *ArtifactStore
	f    *os.File
	tmp  string
	rel  string
	h    interface{ Sum([]byte) []byte }
	w    io.Writer
	size int64
	max  int64
}

// Begin opens a writer for a job's archive. runUsed is the bytes the run's
// earlier archives already hold.
func (s *ArtifactStore) Begin(runID, id string, runUsed int64) (*ArtifactWriter, error) {
	rel := filepath.ToSlash(filepath.Join(sanitizeName(runID), sanitizeName(id)+".tar.gz"))
	full, err := s.resolve(rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, fmt.Errorf("pipeline: create run artifact dir: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("pipeline: create artifact file: %w", err)
	}
	limit := s.lim.MaxBytes
	if s.lim.MaxRunBytes > 0 && (limit <= 0 || s.lim.MaxRunBytes-runUsed < limit) {
		limit = max(s.lim.MaxRunBytes-runUsed, 0)
	}
	h := sha256.New()
	return &ArtifactWriter{s: s, f: f, tmp: f.Name(), rel: rel, h: h, w: io.MultiWriter(f, h), max: limit}, nil
}

// Write stores p, failing with ErrArtifactTooLarge once the cap is passed.
func (a *ArtifactWriter) Write(p []byte) (int, error) {
	if a.max > 0 && a.size+int64(len(p)) > a.max {
		return 0, ErrArtifactTooLarge
	}
	n, err := a.w.Write(p)
	a.size += int64(n)
	return n, err
}

// Size is the bytes written so far.
func (a *ArtifactWriter) Size() int64 { return a.size }

// Abort discards the partial file.
func (a *ArtifactWriter) Abort() {
	_ = a.f.Close()
	_ = os.Remove(a.tmp)
}

// Commit closes the file and moves it into place, returning its relative
// path, size, and sha256.
func (a *ArtifactWriter) Commit() (rel string, size int64, sum string, err error) {
	if err := a.f.Close(); err != nil {
		_ = os.Remove(a.tmp)
		return "", 0, "", fmt.Errorf("pipeline: close artifact file: %w", err)
	}
	full, err := a.s.resolve(a.rel)
	if err != nil {
		_ = os.Remove(a.tmp)
		return "", 0, "", err
	}
	if err := os.Rename(a.tmp, full); err != nil {
		_ = os.Remove(a.tmp)
		return "", 0, "", fmt.Errorf("pipeline: store artifact: %w", err)
	}
	return a.rel, a.size, hex.EncodeToString(a.h.Sum(nil)), nil
}

// Open verifies the stored archive against meta.SHA256 and returns it
// positioned at the start. A mismatch returns ErrArtifactCorrupt.
func (s *ArtifactStore) Open(meta store.PipelineArtifact) (*os.File, error) {
	full, err := s.resolve(meta.StoragePath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full) //nolint:gosec // full comes from resolve(), which confines it to the store directory
	if err != nil {
		return nil, fmt.Errorf("pipeline: open artifact: %w", err)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("pipeline: hash artifact: %w", err)
	}
	if n != meta.SizeBytes || hex.EncodeToString(h.Sum(nil)) != meta.SHA256 {
		_ = f.Close()
		return nil, ErrArtifactCorrupt
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("pipeline: rewind artifact: %w", err)
	}
	return f, nil
}

// Remove deletes a stored archive; a missing file is not an error.
func (s *ArtifactStore) Remove(rel string) error {
	full, err := s.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pipeline: remove artifact: %w", err)
	}
	_ = os.Remove(filepath.Dir(full))
	return nil
}

// Sweep deletes archives past retention and run directories whose run no
// longer exists. It returns how many archives it removed.
func (s *ArtifactStore) Sweep(ctx context.Context, idx ArtifactIndex, runExists func(ctx context.Context, runID string) bool) (int, error) {
	expired, err := idx.ListExpiredPipelineArtifacts(ctx, s.now())
	if err != nil {
		return 0, fmt.Errorf("pipeline: list expired artifacts: %w", err)
	}
	removed := 0
	var errs []error
	for _, a := range expired {
		if err := s.Remove(a.StoragePath); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := idx.DeletePipelineArtifact(ctx, a.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return removed, errors.Join(append(errs, fmt.Errorf("pipeline: read artifact dir: %w", err))...)
	}
	for _, e := range entries {
		if !e.IsDir() || runExists(ctx, e.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			errs = append(errs, fmt.Errorf("pipeline: remove orphan artifact dir: %w", err))
		}
	}
	return removed, errors.Join(errs...)
}

// VerifiedReader streams a stored archive while hashing it, so the sha256
// is checked without reading the file twice. Verify must run after the
// consumer is done: it drains whatever the consumer left unread and
// reports ErrArtifactCorrupt on a size or hash mismatch.
type VerifiedReader struct {
	f    *os.File
	h    hash.Hash
	meta store.PipelineArtifact
	n    int64
	done bool
	err  error
}

func (v *VerifiedReader) Read(p []byte) (int, error) {
	if v.err != nil {
		return 0, v.err
	}
	n, err := v.f.Read(p)
	v.h.Write(p[:n])
	v.n += int64(n)
	if errors.Is(err, io.EOF) {
		v.done = true
		if cerr := v.check(); cerr != nil {
			return n, cerr
		}
	}
	return n, err
}

func (v *VerifiedReader) check() error {
	if v.n != v.meta.SizeBytes || hex.EncodeToString(v.h.Sum(nil)) != v.meta.SHA256 {
		v.err = ErrArtifactCorrupt
	}
	return v.err
}

// Verify finishes the hash and returns ErrArtifactCorrupt on a mismatch.
func (v *VerifiedReader) Verify() error {
	if !v.done && v.err == nil {
		if _, err := io.Copy(io.Discard, v); err != nil {
			return err
		}
	}
	return v.err
}

// Close releases the file.
func (v *VerifiedReader) Close() error { return v.f.Close() }

// OpenVerified opens meta's archive for streaming with the sha256 checked
// as the bytes are read, not before.
func (s *ArtifactStore) OpenVerified(meta store.PipelineArtifact) (*VerifiedReader, error) {
	full, err := s.resolve(meta.StoragePath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full) //nolint:gosec // full comes from resolve(), which confines it to the store directory
	if err != nil {
		return nil, fmt.Errorf("pipeline: open artifact: %w", err)
	}
	return &VerifiedReader{f: f, h: sha256.New(), meta: meta}, nil
}
