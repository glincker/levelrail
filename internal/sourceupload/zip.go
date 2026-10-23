package sourceupload

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// ErrEmptyArchive is returned when a zip holds no files once OS metadata is ignored.
var ErrEmptyArchive = errors.New("sourceupload: archive has no files")

// ZipResult describes a converted zip.
type ZipResult struct {
	Stats
	// HasIndex reports whether index.html sits at the site root after wrapper folders are stripped.
	HasIndex bool
	// StrippedDir is the wrapper folder path removed so the site root is the archive root.
	StrippedDir string
}

type zipEntry struct {
	name string
	file *zip.File
}

func isZipJunk(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if seg == "__MACOSX" {
			return true
		}
	}
	switch path.Base(name) {
	case ".DS_Store", "Thumbs.db":
		return true
	}
	return false
}

// ConvertZip validates a zip and rewrites it as the gzip tar the store keeps.
// Wrapper folders holding the whole site are stripped; symlinks and escaping paths are refused.
func ConvertZip(src io.ReaderAt, size int64, w io.Writer, lim Limits) (ZipResult, error) {
	lim = lim.withDefaults()
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return ZipResult{}, fmt.Errorf("sourceupload: not a valid zip file: %w", err)
	}
	entries, total, err := collectZipEntries(zr, lim)
	if err != nil {
		return ZipResult{}, err
	}
	if len(entries) == 0 {
		return ZipResult{}, ErrEmptyArchive
	}
	res := ZipResult{Stats: Stats{Bytes: total}}
	res.StrippedDir, res.HasIndex = stripWrapper(entries)

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := writeZipEntry(tw, e); err != nil {
			return ZipResult{}, err
		}
		res.Files++
	}
	if err := tw.Close(); err != nil {
		return ZipResult{}, fmt.Errorf("sourceupload: finish archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return ZipResult{}, fmt.Errorf("sourceupload: finish archive: %w", err)
	}
	return res, nil
}

func collectZipEntries(zr *zip.Reader, lim Limits) ([]zipEntry, int64, error) {
	var out []zipEntry
	var total int64
	seen := map[string]bool{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			dir := strings.TrimSuffix(f.Name, "/")
			if _, err := cleanEntry(dir); err != nil && dir != "" {
				return nil, 0, err
			}
			continue
		}
		name, err := cleanEntry(f.Name)
		if err != nil {
			return nil, 0, err
		}
		if isZipJunk(name) {
			continue
		}
		if mode := f.Mode(); mode&fs.ModeSymlink != 0 || !mode.IsRegular() {
			return nil, 0, fmt.Errorf("%w: %q is a link or special file, only regular files are accepted", ErrUnsafeArchive, f.Name)
		}
		if seen[name] {
			return nil, 0, fmt.Errorf("%w: %q appears twice", ErrUnsafeArchive, f.Name)
		}
		seen[name] = true
		if len(out)+1 > lim.MaxFiles {
			return nil, 0, fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.MaxFiles)
		}
		declared := int64(f.UncompressedSize64) //nolint:gosec // overflow shows up as a negative value, rejected below
		if declared < 0 || declared > lim.MaxFileBytes || total+declared > lim.MaxBytes {
			return nil, 0, fmt.Errorf("%w: expands past %d bytes", ErrTooLarge, lim.MaxBytes)
		}
		total += declared
		out = append(out, zipEntry{name: name, file: f})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, total, nil
}

// stripWrapper removes single wrapper folders until index.html is at the root.
// When no level has an index.html the names are left untouched.
func stripWrapper(entries []zipEntry) (stripped string, hasIndex bool) {
	orig := make([]string, len(entries))
	for i, e := range entries {
		orig[i] = e.name
	}
	var removed []string
	for {
		if rootHasIndex(entries) {
			return strings.Join(removed, "/"), true
		}
		first, _, ok := strings.Cut(entries[0].name, "/")
		if !ok {
			break
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.name, first+"/") {
				first = ""
				break
			}
		}
		if first == "" {
			break
		}
		for i := range entries {
			entries[i].name = strings.TrimPrefix(entries[i].name, first+"/")
		}
		removed = append(removed, first)
	}
	for i := range entries {
		entries[i].name = orig[i]
	}
	return "", false
}

func rootHasIndex(entries []zipEntry) bool {
	for _, e := range entries {
		if e.name == "index.html" {
			return true
		}
	}
	return false
}

func writeZipEntry(tw *tar.Writer, e zipEntry) error {
	want := int64(e.file.UncompressedSize64) //nolint:gosec // bounded by MaxFileBytes in collectZipEntries
	hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: want, ModTime: time.Unix(0, 0)}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("sourceupload: write %q: %w", e.name, err)
	}
	rc, err := e.file.Open()
	if err != nil {
		return fmt.Errorf("sourceupload: read %q: %w", e.name, err)
	}
	defer func() { _ = rc.Close() }()
	n, err := io.Copy(tw, io.LimitReader(rc, want+1))
	if err != nil && !errors.Is(err, tar.ErrWriteTooLong) {
		return fmt.Errorf("sourceupload: copy %q: %w", e.name, err)
	}
	if n != want || errors.Is(err, tar.ErrWriteTooLong) {
		return sizeCheck(e.name, n, want)
	}
	return nil
}
