package sourceupload

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafeArchive is returned for an entry that could escape the extraction root or is not a plain file.
var ErrUnsafeArchive = errors.New("sourceupload: unsafe archive entry")

// Limits cap what an archive may expand to. Zero fields use the defaults.
type Limits struct {
	MaxFiles     int
	MaxBytes     int64
	MaxFileBytes int64
}

const (
	defaultMaxFiles     = 50000
	defaultMaxBytes     = 2 << 30
	defaultMaxFileBytes = 1 << 30
)

func (l Limits) withDefaults() Limits {
	if l.MaxFiles <= 0 {
		l.MaxFiles = defaultMaxFiles
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = defaultMaxBytes
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = defaultMaxFileBytes
	}
	return l
}

// Stats describes an archive's contents.
type Stats struct {
	Files int
	Bytes int64
}

// Inspect validates every entry and counts files and bytes without writing anything.
func Inspect(r io.Reader, lim Limits) (Stats, error) {
	return walk(r, "", lim)
}

// Extract unpacks r under dest, refusing anything but regular files and directories.
func Extract(r io.Reader, dest string, lim Limits) (Stats, error) {
	if dest == "" {
		return Stats{}, errors.New("sourceupload: extract destination is required")
	}
	return walk(r, dest, lim)
}

func cleanEntry(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) || strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("%w: %q", ErrUnsafeArchive, name)
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafeArchive, name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: parent reference in %q", ErrUnsafeArchive, name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrUnsafeArchive, name)
	}
	return clean, nil
}

func walk(r io.Reader, dest string, lim Limits) (Stats, error) {
	lim = lim.withDefaults()
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Stats{}, fmt.Errorf("sourceupload: not a gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	var st Stats
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return st, nil
		}
		if err != nil {
			return st, fmt.Errorf("sourceupload: read archive: %w", err)
		}
		name, err := cleanEntry(hdr.Name)
		if err != nil {
			return st, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if dest != "" {
				if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(name)), 0o750); err != nil {
					return st, fmt.Errorf("sourceupload: create directory %q: %w", name, err)
				}
			}
		case tar.TypeReg:
			if st.Files+1 > lim.MaxFiles {
				return st, fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.MaxFiles)
			}
			if hdr.Size < 0 || hdr.Size > lim.MaxFileBytes || st.Bytes+hdr.Size > lim.MaxBytes {
				return st, fmt.Errorf("%w: expands past %d bytes", ErrTooLarge, lim.MaxBytes)
			}
			n, err := copyEntry(tr, dest, name, hdr.Size, hdr.Mode)
			if err != nil {
				return st, err
			}
			st.Files++
			st.Bytes += n
		default:
			return st, fmt.Errorf("%w: %q has type %q, only regular files are accepted", ErrUnsafeArchive, hdr.Name, string(hdr.Typeflag))
		}
	}
}

// copyEntry reads exactly size bytes (a stream that outruns its header fails) and writes them when dest is set.
func copyEntry(tr *tar.Reader, dest, name string, size, mode int64) (int64, error) {
	src := io.LimitReader(tr, size+1)
	if dest == "" {
		n, err := io.Copy(io.Discard, src)
		if err != nil {
			return n, fmt.Errorf("sourceupload: read %q: %w", name, err)
		}
		return n, sizeCheck(name, n, size)
	}
	target := filepath.Join(dest, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return 0, fmt.Errorf("sourceupload: create parent of %q: %w", name, err)
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) //nolint:gosec // target is a cleaned relative path joined under dest
	if err != nil {
		return 0, fmt.Errorf("sourceupload: create %q: %w", name, err)
	}
	n, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil {
		return n, fmt.Errorf("sourceupload: write %q: %w", name, copyErr)
	}
	if closeErr != nil {
		return n, fmt.Errorf("sourceupload: close %q: %w", name, closeErr)
	}
	return n, sizeCheck(name, n, size)
}

func sizeCheck(name string, got, want int64) error {
	if got != want {
		return fmt.Errorf("%w: %q size does not match its header", ErrUnsafeArchive, name)
	}
	return nil
}
