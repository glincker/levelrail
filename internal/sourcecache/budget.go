package sourcecache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// ErrOverBudget means a fetched mirror passed the caller's byte budget; the
// mirror was deleted so it cannot keep filling the disk.
var ErrOverBudget = errors.New("sourcecache: mirror is larger than the fetch budget")

type maxBytesKey struct{}

// WithMaxMirrorBytes caps the mirror one Checkout may leave on disk. go-git
// cannot stop a fetch mid stream, so the cap is enforced right after it.
func WithMaxMirrorBytes(ctx context.Context, n int64) context.Context {
	return context.WithValue(ctx, maxBytesKey{}, n)
}

func maxBytesOf(ctx context.Context) int64 {
	n, _ := ctx.Value(maxBytesKey{}).(int64)
	return n
}

// MirrorRoot is the directory mirrors live under, "" when each fetch is a throwaway.
func (c *Cache) MirrorRoot() string { return c.cfg.Dir }

// overBudget deletes mirrorDir after release when it passed the budget and no
// other checkout holds it.
func (c *Cache) overBudget(ctx context.Context, mirrorDir string, release func()) bool {
	limit := maxBytesOf(ctx)
	if limit <= 0 || dirSize(mirrorDir) <= limit {
		return false
	}
	release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if st := c.mirrors[filepath.Base(mirrorDir)]; st == nil || st.users == 0 {
		_ = os.RemoveAll(mirrorDir)
	}
	return true
}
