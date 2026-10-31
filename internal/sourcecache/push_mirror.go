package sourcecache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
)

// ErrNoMirrorDir means the cache keeps no mirror, so pushed objects would be lost.
var ErrNoMirrorDir = errors.New("sourcecache: no mirror directory configured")

// MirrorSize is the bytes on disk the mirror for repoURL holds, 0 when absent.
func (c *Cache) MirrorSize(repoURL string) int64 {
	if c.cfg.Dir == "" {
		return 0
	}
	return dirSize(filepath.Join(c.cfg.Dir, mirrorKey(repoURL, "")))
}

// ResetMirror deletes the mirror for repoURL. Callers must hold no OpenMirror
// handle for it.
func (c *Cache) ResetMirror(repoURL string) error {
	if c.cfg.Dir == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(c.cfg.Dir, mirrorKey(repoURL, ""))); err != nil {
		return fmt.Errorf("sourcecache: reset mirror: %w", err)
	}
	return nil
}

// OpenMirror opens, creating if needed, the bare mirror for repoURL and marks
// it in use until release runs. A pushed commit stored here is later checked
// out by hash with no network call. size reports the mirror's bytes on disk.
func (c *Cache) OpenMirror(repoURL string) (repo *git.Repository, size func() int64, release func(), err error) {
	if c.cfg.Dir == "" {
		return nil, nil, nil, ErrNoMirrorDir
	}
	dir, rel, err := c.acquire(repoURL, "")
	if err != nil {
		return nil, nil, nil, err
	}
	repo, err = c.openMirror(dir)
	if err != nil {
		rel()
		return nil, nil, nil, err
	}
	return repo, func() int64 { return dirSize(dir) }, func() { c.touch(dir); rel() }, nil
}
