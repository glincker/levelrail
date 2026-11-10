package build

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// InlineDockerfileName is where an inline Dockerfile lands in its build context.
const InlineDockerfileName = "Dockerfile"

// ErrUnsafeInlinePath marks an extra file name that could escape or shadow the build context.
var ErrUnsafeInlinePath = errors.New("unsafe inline file path")

// InlineFile is one extra build context file sent with an inline Dockerfile.
type InlineFile struct {
	Path    string
	Content string
}

// ValidateInlinePath accepts only clean, relative, slash-separated names
// inside the context that do not replace the Dockerfile itself.
func ValidateInlinePath(p string) error {
	switch {
	case p == "", len(p) > 255, strings.ContainsAny(p, "\\\x00:"), strings.HasPrefix(p, "/"):
		return fmt.Errorf("%w: %q", ErrUnsafeInlinePath, p)
	case path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../"):
		return fmt.Errorf("%w: %q", ErrUnsafeInlinePath, p)
	case p == InlineDockerfileName:
		return fmt.Errorf("%w: %q is the inline Dockerfile itself", ErrUnsafeInlinePath, p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return fmt.Errorf("%w: %q", ErrUnsafeInlinePath, p)
		}
	}
	return nil
}

// ValidateInlineFileSet checks every name and refuses names that collide on a
// case-insensitive or Unicode-normalizing filesystem, or a file that is also a parent directory.
func ValidateInlineFileSet(paths []string) error {
	fold := func(p string) string { return strings.ToLower(norm.NFC.String(p)) }
	files := map[string]string{fold(InlineDockerfileName): InlineDockerfileName}
	for _, p := range paths {
		if err := ValidateInlinePath(p); err != nil {
			return err
		}
		if prev, ok := files[fold(p)]; ok {
			return fmt.Errorf("%w: %q collides with %q", ErrUnsafeInlinePath, p, prev)
		}
		files[fold(p)] = p
	}
	for _, p := range paths {
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			if prev, ok := files[fold(d)]; ok {
				return fmt.Errorf("%w: %q needs %q to be a directory", ErrUnsafeInlinePath, p, prev)
			}
		}
	}
	return nil
}

// WriteInlineContext writes the Dockerfile and files under dir through an
// os.Root, so no name or pre-existing symlink can place a byte outside dir.
// Files are created exclusively with 0600 and directories with 0700.
func WriteInlineContext(dir, dockerfile string, files []InlineFile) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open inline build context: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err := writeRootFile(root, InlineDockerfileName, dockerfile); err != nil {
		return err
	}
	for _, f := range files {
		if err := ValidateInlinePath(f.Path); err != nil {
			return err
		}
		if d := path.Dir(f.Path); d != "." {
			if err := mkdirAllRoot(root, d); err != nil {
				return err
			}
		}
		if err := writeRootFile(root, f.Path, f.Content); err != nil {
			return err
		}
	}
	return nil
}

// MaterializeInlineContext writes an inline source into a fresh temp dir; cleanup removes it.
func MaterializeInlineContext(dockerfile string, files []InlineFile) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "inline-src-*")
	if err != nil {
		return "", nil, fmt.Errorf("create inline build context: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	if err := WriteInlineContext(dir, dockerfile, files); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func mkdirAllRoot(root *os.Root, d string) error {
	cur := ""
	for _, seg := range strings.Split(d, "/") {
		cur = path.Join(cur, seg)
		fi, err := root.Lstat(cur)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%w: %q is a symlink", ErrUnsafeInlinePath, cur)
		case err == nil && !fi.IsDir():
			return fmt.Errorf("%w: %q is not a directory", ErrUnsafeInlinePath, cur)
		case err == nil:
			continue
		case !errors.Is(err, os.ErrNotExist):
			return fmt.Errorf("inline context dir %q: %w", cur, err)
		}
		if err := root.Mkdir(cur, 0o700); err != nil {
			return fmt.Errorf("inline context dir %q: %w", cur, err)
		}
	}
	return nil
}

func writeRootFile(root *os.Root, name, content string) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %q already exists", ErrUnsafeInlinePath, name)
		}
		return fmt.Errorf("write inline file %q: %w", name, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write inline file %q: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write inline file %q: %w", name, err)
	}
	return nil
}
