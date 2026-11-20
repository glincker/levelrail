package pages

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var siteNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CacheKey derives a cache key from lockfile contents, so a changed lockfile starts a fresh cache.
func CacheKey(lockfiles ...[]byte) string {
	h := sha256.New()
	for _, b := range lockfiles {
		sum := sha256.Sum256(b)
		h.Write(sum[:])
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// CacheDir returns the per-site cache directory under root. The site name is validated so one
// site can never address another site's cache.
func CacheDir(root, site string) (string, error) {
	if !siteNameRe.MatchString(site) {
		return "", errors.New("pages: invalid site name for build cache")
	}
	return filepath.Join(root, site), nil
}

// EvictOldest removes the oldest key directories of one site until its cache fits limit bytes.
// The key just used is spared so a build never deletes the cache it is about to read.
func EvictOldest(siteDir, keep string, limit int64) error {
	entries, err := os.ReadDir(siteDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type item struct {
		name string
		size int64
		mod  int64
	}
	var items []item
	var total int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		it := item{name: e.Name()}
		if info, err := e.Info(); err == nil {
			it.mod = info.ModTime().UnixNano()
		}
		_ = filepath.WalkDir(filepath.Join(siteDir, e.Name()), func(_ string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, ierr := d.Info(); ierr == nil {
					it.size += info.Size()
				}
			}
			return nil
		})
		total += it.size
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod < items[j].mod })
	for _, it := range items {
		if total <= limit {
			break
		}
		if it.name == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(siteDir, it.name)); err != nil {
			return err
		}
		total -= it.size
	}
	return nil
}
