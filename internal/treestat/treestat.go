// Package treestat counts regular files and bytes under a directory. The source
// agent and the control plane both use it so the two sides of a comparison
// measure identically.
package treestat

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ErrLimit means a walk stopped because the tree was larger or deeper than allowed.
var ErrLimit = errors.New("treestat: tree exceeds the scan limits")

// Limits bound a walk; a zero field means unlimited.
type Limits struct {
	MaxEntries int64
	MaxDepth   int
}

// Stat is the file count and total size of a tree.
type Stat struct {
	Files int64
	Bytes int64
}

// Walk totals regular files under root. Symlinks and special files are not
// followed or counted. The first unreadable entry is returned with the totals
// gathered so far.
func Walk(ctx context.Context, root string) (Stat, error) {
	return WalkLimited(ctx, root, Limits{})
}

// WalkLimited is Walk that fails with ErrLimit instead of walking past lim.
func WalkLimited(ctx context.Context, root string, lim Limits) (Stat, error) {
	var st Stat
	var entries int64
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))
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
		entries++
		if lim.MaxEntries > 0 && entries > lim.MaxEntries {
			return ErrLimit
		}
		if lim.MaxDepth > 0 && strings.Count(filepath.Clean(path), string(filepath.Separator))-rootDepth > lim.MaxDepth {
			return ErrLimit
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
