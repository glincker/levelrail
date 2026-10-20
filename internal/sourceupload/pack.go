package sourceupload

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrTooLarge is returned when a directory or archive exceeds its size cap.
var ErrTooLarge = errors.New("sourceupload: source exceeds the size limit")

// Options tune Pack.
type Options struct {
	// Include re-adds paths the default and .gitignore rules would exclude, for example ".env".
	Include []string
	// MaxBytes caps the total uncompressed size of the files; 0 means no cap.
	MaxBytes int64
}

// File is one entry that will be uploaded.
type File struct {
	Path string
	Size int64
	Mode fs.FileMode
}

// Plan is the resolved list of files, shown to the user before anything is sent.
type Plan struct {
	Root  string
	Files []File
	Total int64
}

// Largest returns up to n files, biggest first.
func (p Plan) Largest(n int) []File {
	out := append([]File(nil), p.Files...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Scan walks root applying the ignore rules. Symlinks and special files are skipped.
func Scan(ctx context.Context, root string, opts Options) (Plan, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("sourceupload: resolve %q: %w", root, err)
	}
	set := newIgnoreSet(opts.Include)
	plan := Plan{Root: abs}
	walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		rel, rerr := filepath.Rel(abs, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			set.addFile(filepath.Join(p, ".gitignore"), nil)
			return nil
		}
		rel = filepath.ToSlash(rel)
		if set.excluded(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			set.addFile(filepath.Join(p, ".gitignore"), strings.Split(rel, "/"))
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		plan.Files = append(plan.Files, File{Path: rel, Size: info.Size(), Mode: info.Mode().Perm()})
		plan.Total += info.Size()
		if opts.MaxBytes > 0 && plan.Total > opts.MaxBytes {
			return fmt.Errorf("%w: more than %d bytes", ErrTooLarge, opts.MaxBytes)
		}
		return nil
	})
	if walkErr != nil {
		return Plan{}, walkErr
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

// WriteArchive writes the plan as a gzip tar. Entries are sorted with zeroed
// times, owners and normalised modes so the same tree always hashes the same.
func (p Plan) WriteArchive(ctx context.Context, w io.Writer) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("sourceupload: gzip writer: %w", err)
	}
	tw := tar.NewWriter(gz)
	for _, f := range p.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		mode := int64(0o644)
		if f.Mode&0o111 != 0 {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: f.Path, Size: f.Size, Mode: mode, Format: tar.FormatPAX}); err != nil {
			return fmt.Errorf("sourceupload: write header %q: %w", f.Path, err)
		}
		if err := copyFile(tw, filepath.Join(p.Root, filepath.FromSlash(f.Path)), f.Size); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("sourceupload: close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("sourceupload: close gzip: %w", err)
	}
	return nil
}

func copyFile(w io.Writer, path string, size int64) error {
	in, err := os.Open(path) //nolint:gosec // path comes from Scan of the user's own directory
	if err != nil {
		return fmt.Errorf("sourceupload: open %q: %w", path, err)
	}
	defer func() { _ = in.Close() }()
	n, err := io.Copy(w, io.LimitReader(in, size))
	if err != nil {
		return fmt.Errorf("sourceupload: read %q: %w", path, err)
	}
	if n != size {
		return fmt.Errorf("sourceupload: %q changed while packing", path)
	}
	return nil
}
