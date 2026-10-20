package build

// This file: the wire format a dispatched build's context travels in.
// Uncompressed tar, deliberately: the image tar coming back is the far
// larger transfer, and adding compression to only one direction buys
// little while costing CPU on the control plane, which is the machine
// dedicated build nodes exist to keep free.

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrUnsafeContextPath is returned by UntarContext for an entry whose
// name escapes the destination directory. A build context arrives over
// the agent transport from an authenticated control plane, but an
// extractor that trusts entry names is a file-overwrite primitive
// regardless of who is on the other end.
var ErrUnsafeContextPath = errors.New("build: build context entry escapes the destination directory")

// TarContext writes dir's contents to w as a tar stream: regular files,
// directories, and symlinks, with their permission bits. Anything else
// (sockets, devices, named pipes) is skipped, since no build context
// meaningfully needs one and Docker's own context upload skips them too.
func TarContext(ctx context.Context, dir string, w io.Writer) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("build: resolve build context %q: %w", dir, err)
	}

	tw := tar.NewWriter(w)
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(path); err != nil {
				return err
			}
		} else if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}

		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
		}
		// Ownership is dropped rather than carried: the build node's own
		// uid/gid namespace has nothing to do with the control plane's,
		// and BuildKit only ever reads these files.
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path) //nolint:gosec // path comes from walking the caller's own build context
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if walkErr != nil {
		_ = tw.Close()
		return fmt.Errorf("build: archive build context %q: %w", dir, walkErr)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("build: archive build context %q: %w", dir, err)
	}
	return nil
}

// UntarContext extracts a TarContext stream into dir, which must already
// exist. Entry names that escape dir fail the whole extraction with
// ErrUnsafeContextPath rather than being skipped: a context that cannot
// be reproduced faithfully must not be built. Every write goes through
// an os.Root, so even a symlink the checks below miss cannot redirect a
// write outside dir.
func UntarContext(ctx context.Context, r io.Reader, dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("build: open build context destination %q: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	symlinks := map[string]bool{}
	tr := tar.NewReader(r)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("build: read build context stream: %w", err)
		}

		name := filepath.Clean(filepath.FromSlash(header.Name))
		if header.Name == "" || !filepath.IsLocal(name) || underSymlink(symlinks, name) {
			return fmt.Errorf("%w: %q", ErrUnsafeContextPath, header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, os.FileMode(header.Mode).Perm()|0o700); err != nil { //nolint:gosec // header.Mode is masked to permission bits
				return fmt.Errorf("build: create build context dir: %w", err)
			}
		case tar.TypeSymlink:
			if err := writeContextSymlink(root, name, header.Linkname); err != nil {
				return err
			}
			symlinks[name] = true
		case tar.TypeReg:
			if err := writeContextFile(root, name, tr, os.FileMode(header.Mode).Perm()); err != nil { //nolint:gosec // header.Mode is masked to permission bits
				return err
			}
		default:
			// TarContext never produces anything else (hard links
			// included); anything that shows up here is not a build input
			// worth reproducing.
			continue
		}
	}
}

// underSymlink reports whether any parent of name is a symlink extracted
// earlier. TarContext never emits such an entry, and allowing one would
// make the lexical checks here disagree with where the write really lands.
func underSymlink(symlinks map[string]bool, name string) bool {
	for dir := filepath.Dir(name); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if symlinks[dir] {
			return true
		}
	}
	return false
}

func writeContextFile(root *os.Root, name string, r io.Reader, mode os.FileMode) error {
	if err := root.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return fmt.Errorf("build: create build context dir: %w", err)
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("build: create build context file: %w", err)
	}
	//nolint:gosec // a build context is operator-supplied source, not an untrusted upload with a size budget
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("build: write build context file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("build: close build context file: %w", err)
	}
	return nil
}

// writeContextSymlink refuses an absolute link or one whose target,
// resolved from the link's own directory, leaves the context root.
func writeContextSymlink(root *os.Root, name, linkname string) error {
	if linkname == "" || filepath.IsAbs(linkname) || !filepath.IsLocal(filepath.Join(filepath.Dir(name), filepath.FromSlash(linkname))) {
		return fmt.Errorf("%w: symlink to %q", ErrUnsafeContextPath, linkname)
	}
	if err := root.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return fmt.Errorf("build: create build context dir: %w", err)
	}
	if err := root.Symlink(linkname, name); err != nil {
		return fmt.Errorf("build: create build context symlink: %w", err)
	}
	return nil
}
