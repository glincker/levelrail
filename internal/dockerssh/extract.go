package dockerssh

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Limits bound one extraction. Zero means unlimited.
type Limits struct {
	MaxBytes int64
	MaxFiles int
}

// ErrLimit is returned when an archive passes a size or file count cap.
var ErrLimit = errors.New("dockerssh: archive passes the configured limit")

// ExtractStats reports what an extraction wrote.
type ExtractStats struct {
	Files   int   `json:"files"`
	Dirs    int   `json:"dirs"`
	Bytes   int64 `json:"bytes"`
	Skipped int   `json:"skipped"`
	// Warnings names entries that were left out and why, capped.
	Warnings []string `json:"warnings,omitempty"`
}

const maxWarnings = 20

func (st *ExtractStats) skip(name, why string) {
	st.Skipped++
	if len(st.Warnings) < maxWarnings {
		st.Warnings = append(st.Warnings, name+": "+why)
	}
}

// ExtractArchive unpacks a container archive of one directory into dst. The
// archive's first path component (the directory's own name) is stripped.
// Names that leave dst, symlinks that could leave it, hard links and device
// nodes are never created, and no entry is written through a symlink.
func ExtractArchive(r io.Reader, dst string, lim Limits) (ExtractStats, error) {
	var st ExtractStats
	root, err := filepath.Abs(dst)
	if err != nil {
		return st, fmt.Errorf("resolve target: %w", err)
	}
	if fi, err := os.Lstat(root); err != nil || !fi.IsDir() {
		return st, fmt.Errorf("the target %s is not a directory", root)
	}
	if lim.MaxBytes > 0 {
		r = io.LimitReader(r, lim.MaxBytes+lim.MaxBytes/4+(1<<20))
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return st, nil
		}
		if err != nil {
			return st, fmt.Errorf("read archive: %w", err)
		}
		if slices.Contains(strings.Split(hdr.Name, "/"), "..") {
			return st, fmt.Errorf("archive entry %q escapes the target", hdr.Name)
		}
		rel, ok := stripFirst(hdr.Name)
		if !ok {
			continue
		}
		if rel == "" {
			continue
		}
		if !safeRel(rel) {
			return st, fmt.Errorf("archive entry %q escapes the target", hdr.Name)
		}
		if lim.MaxFiles > 0 && st.Files+st.Dirs >= lim.MaxFiles {
			return st, fmt.Errorf("%w: more than %d entries", ErrLimit, lim.MaxFiles)
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := parentsAreDirs(root, target); err != nil {
			return st, fmt.Errorf("archive entry %q: %w", hdr.Name, err)
		}
		// MkdirAll and chmod follow a symlink leaf; regular files remove it first instead.
		if hdr.Typeflag == tar.TypeDir {
			if fi, lerr := os.Lstat(target); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
				return st, fmt.Errorf("archive entry %q would be written through a symlink", hdr.Name)
			}
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil { //nolint:gosec // mode is tightened to the archive's below
				return st, fmt.Errorf("create %s: %w", rel, err)
			}
			applyMeta(target, hdr, false)
			_ = os.Chmod(target, permBits(hdr.Mode)|0o700)
			st.Dirs++
		case tar.TypeReg:
			n, err := writeFile(tr, target, hdr, lim, st.Bytes)
			st.Bytes += n
			if err != nil {
				return st, fmt.Errorf("write %s: %w", rel, err)
			}
			st.Files++
		case tar.TypeSymlink:
			if !symlinkStaysInside(rel, hdr.Linkname) || !linkResolvesInside(root, target, hdr.Linkname) {
				st.skip(rel, "symlink target is absolute or leaves the volume")
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil { //nolint:gosec // parent of an archive entry inside the target
				return st, fmt.Errorf("create parent of %s: %w", rel, err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil && !errors.Is(err, os.ErrExist) {
				return st, fmt.Errorf("create symlink %s: %w", rel, err)
			}
			applyMeta(target, hdr, true)
			st.Files++
		default:
			st.skip(rel, "entry type is not copied (hard link, device or fifo)")
		}
	}
}

func stripFirst(name string) (string, bool) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "" || name == "." {
		return "", false
	}
	_, rest, found := strings.Cut(name, "/")
	if !found {
		return "", true
	}
	return rest, true
}

func safeRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, 0) {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func symlinkStaysInside(rel, link string) bool {
	if link == "" || strings.HasPrefix(link, "/") || strings.ContainsRune(link, 0) {
		return false
	}
	joined := path.Clean(path.Join(path.Dir(rel), link))
	return joined != ".." && !strings.HasPrefix(joined, "../") && !path.IsAbs(joined)
}

// linkResolvesInside walks link through symlinks already on disk, because a
// lexical clean cannot see that "d/l/.." leaves the root once d/l is a link.
func linkResolvesInside(root, target, link string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	cur, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil || !within(realRoot, cur) {
		return false
	}
	for _, seg := range strings.Split(link, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
		default:
			cur = filepath.Join(cur, seg)
			if fi, lerr := os.Lstat(cur); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
				resolved, rerr := filepath.EvalSymlinks(cur)
				if rerr != nil {
					return false
				}
				cur = resolved
			}
		}
		if !within(realRoot, cur) {
			return false
		}
	}
	return true
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// parentsAreDirs refuses to write below any component that is a symlink or
// a file, so a link planted earlier in the archive cannot redirect a write.
func parentsAreDirs(root, target string) error {
	dir := filepath.Dir(target)
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return errors.New("entry escapes the target")
	}
	cur := root
	if rel == "." {
		return nil
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(cur, 0o700); err != nil {
				return fmt.Errorf("create %s: %w", seg, err)
			}
		case err != nil:
			return err
		case !fi.IsDir():
			return fmt.Errorf("%s is a symlink or file, not a directory", seg)
		}
	}
	return nil
}

func writeFile(r io.Reader, target string, hdr *tar.Header, lim Limits, written int64) (int64, error) {
	if lim.MaxBytes > 0 && written+hdr.Size > lim.MaxBytes {
		return 0, fmt.Errorf("%w: more than %d bytes", ErrLimit, lim.MaxBytes)
	}
	_ = os.Remove(target)
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollowFlag, 0o600) //nolint:gosec // target is checked to sit inside the volume with no symlink parent
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, hdr.Size))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
	}
	applyMeta(target, hdr, false)
	return n, nil
}

func permBits(mode int64) os.FileMode {
	return os.FileMode(mode & 0o777) //nolint:gosec // masked to the nine permission bits
}

func applyMeta(target string, hdr *tar.Header, isLink bool) {
	_ = os.Lchown(target, hdr.Uid, hdr.Gid)
	if isLink {
		return
	}
	_ = os.Chmod(target, permBits(hdr.Mode))
	if !hdr.ModTime.IsZero() {
		_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
	}
}
