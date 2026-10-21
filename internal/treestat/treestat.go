// Package treestat counts regular files and bytes under a directory. The source
// agent and the control plane both use it so the two sides of a comparison
// measure identically.
package treestat

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
)

// Stat is the file count and total size of a tree.
type Stat struct {
	Files int64
	Bytes int64
}

// Walk totals regular files under root. Symlinks and special files are not
// followed or counted. The first unreadable entry is returned with the totals
// gathered so far.
func Walk(ctx context.Context, root string) (Stat, error) {
	var st Stat
	var firstErr error
	note := func(path string, err error) {
		if firstErr == nil {
			firstErr = fmt.Errorf("treestat: %s: %w", path, err)
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			note(path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			note(path, ierr)
			return nil
		}
		st.Files++
		st.Bytes += info.Size()
		return nil
	})
	if err != nil {
		return st, fmt.Errorf("treestat: walk %s: %w", root, err)
	}
	return st, firstErr
}
