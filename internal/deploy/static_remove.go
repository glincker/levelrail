package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnsafeStaticName is returned when a site name could address a path
// outside the static root.
var ErrUnsafeStaticName = errors.New("deploy: static site name is not a single safe path segment")

// RemoveStaticTree deletes <root>/<name>, the whole published tree of one
// site. It never touches anything outside root: the name must be one local
// segment, a symlink at the site path is unlinked rather than followed, and a
// site path that resolves outside the resolved root is refused. A missing
// tree is not an error.
func RemoveStaticTree(root, name string) error {
	if root == "" {
		return errors.New("deploy: static root directory is not configured")
	}
	if name == "" || name == "." || !filepath.IsLocal(name) || filepath.Base(name) != name {
		return fmt.Errorf("%w: %q", ErrUnsafeStaticName, name)
	}
	target := filepath.Join(root, name)
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deploy: stat static tree %q: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("deploy: unlink static tree link %q: %w", target, err)
		}
		return nil
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("deploy: resolve static root %q: %w", root, err)
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("deploy: resolve static tree parent for %q: %w", target, err)
	}
	if realParent != realRoot {
		return fmt.Errorf("deploy: static tree %q resolves outside the static root", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("deploy: remove static tree %q: %w", target, err)
	}
	return nil
}
