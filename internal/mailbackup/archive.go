package mailbackup

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

// scanDir counts regular files and bytes for the manifest.
func scanDir(dir string) (int, int64, error) {
	var files int
	var bytes int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes, err
}

// writeArchive streams dir as tar, then zstd, then age into w.
func writeArchive(dir string, w io.Writer, recipients []age.Recipient) error {
	aw, err := age.Encrypt(w, recipients...)
	if err != nil {
		return err
	}
	zw, err := zstd.NewWriter(aw)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(zw)
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
			return err
		}
		// Hardlinks show up as regular files, so restore needs no hardlinks.
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			h.Name += "/"
		}
		if err := tw.WriteHeader(h); err != nil || d.IsDir() {
			return err
		}
		f, err := os.Open(p) //nolint:gosec // p is a walked path under the staging dir
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if walkErr != nil {
		return walkErr
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return aw.Close()
}

// readArchive decrypts and unpacks r, calling fn for each regular file or dir.
func readArchive(r io.Reader, ids []age.Identity, fn func(h *tar.Header, body io.Reader) error) error {
	ar, err := age.Decrypt(r, ids...)
	if err != nil {
		return err
	}
	zr, err := zstd.NewReader(ar)
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(h, tr); err != nil {
			return err
		}
	}
}

// safeJoin refuses entries that would escape root.
func safeJoin(root, name string) (string, error) {
	p := filepath.Join(root, filepath.FromSlash(name))
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes the restore dir", name)
	}
	return p, nil
}
