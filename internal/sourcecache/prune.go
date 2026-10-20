package sourcecache

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type mirrorInfo struct {
	name     string
	lastUsed time.Time
	size     int64
}

// Prune removes mirrors unused for longer than MaxAge, then the least
// recently used ones until the total fits MaxBytes. Mirrors with a live
// checkout are never removed. Order is (last used, name), so the same
// directory state always prunes the same way. It returns the removed names.
func (c *Cache) Prune(now time.Time) ([]string, error) {
	if c.cfg.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(c.cfg.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	infos := make([]mirrorInfo, 0, len(entries))
	var total int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.cfg.Dir, e.Name())
		mi := mirrorInfo{name: e.Name(), lastUsed: lastUsed(dir), size: dirSize(dir)}
		total += mi.size
		infos = append(infos, mi)
	}
	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].lastUsed.Equal(infos[j].lastUsed) {
			return infos[i].lastUsed.Before(infos[j].lastUsed)
		}
		return infos[i].name < infos[j].name
	})
	var removed []string
	for _, mi := range infos {
		expired := now.Sub(mi.lastUsed) > c.cfg.MaxAge
		if !expired && total <= c.cfg.MaxBytes {
			continue
		}
		if c.busy(mi.name) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.cfg.Dir, mi.name)); err != nil {
			return removed, err
		}
		total -= mi.size
		removed = append(removed, mi.name)
	}
	return removed, nil
}

func (c *Cache) busy(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.mirrors[key]
	return st != nil && st.users > 0
}

func (c *Cache) pruneQuietly() {
	if removed, err := c.Prune(c.now()); err != nil {
		c.log.Warn("sourcecache: prune failed", "error", err.Error())
	} else if len(removed) > 0 {
		c.log.Info("sourcecache: pruned mirrors", "count", len(removed))
	}
}

func lastUsed(dir string) time.Time {
	if st, err := os.Stat(filepath.Join(dir, lastUsedFile)); err == nil {
		return st.ModTime()
	}
	if st, err := os.Stat(dir); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			n += info.Size()
		}
		return nil
	})
	return n
}
