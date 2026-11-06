package build

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bkclient "github.com/moby/buildkit/client"
)

// AppCacheUsage is the size of the BuildKit cache mounts that belong to one app.
type AppCacheUsage struct {
	Bytes      int64
	Records    int
	InUse      bool
	LastUsedAt time.Time
}

// AppCachePruneResult is what a clear removed.
type AppCachePruneResult struct {
	FreedBytes int64
	Removed    int
}

// ErrAppCacheNoBuilder is returned when no BuildKit client is available.
var ErrAppCacheNoBuilder = errors.New("build: no BuildKit client")

// ErrAppCacheInUse is returned when a cache mount of the app is held by a running build.
var ErrAppCacheInUse = errors.New("build: app cache is in use by a running build")

// cacheMountRecordType is the BuildKit record type of a cache mount.
const cacheMountRecordType = "exec.cachemount"

// AppCacheRecords picks the cache mount records of cacheID. BuildKit writes
// `with id "<key>"` into the description, and Railpack keys are `<id>-<name>`.
// siblings are other apps' ids: a record that fits a longer sibling id (app
// "web" next to "web-api") belongs to that sibling and is left alone.
func AppCacheRecords(records []*bkclient.UsageInfo, cacheID string, siblings []string) []*bkclient.UsageInfo {
	if cacheID == "" {
		return nil
	}
	var out []*bkclient.UsageInfo
	for _, r := range records {
		if r == nil || string(r.RecordType) != cacheMountRecordType {
			continue
		}
		key, ok := cacheMountKey(r.Description)
		if !ok || !strings.HasPrefix(key, cacheID+"-") {
			continue
		}
		if ownedBySibling(key, cacheID, siblings) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func ownedBySibling(key, cacheID string, siblings []string) bool {
	for _, s := range siblings {
		if s != cacheID && len(s) > len(cacheID) && strings.HasPrefix(s, cacheID+"-") && strings.HasPrefix(key, s+"-") {
			return true
		}
	}
	return false
}

func cacheMountKey(desc string) (string, bool) {
	const marker = ` with id "`
	i := strings.LastIndex(desc, marker)
	if i < 0 {
		return "", false
	}
	rest := desc[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// SummarizeAppCache totals the records picked by AppCacheRecords.
func SummarizeAppCache(recs []*bkclient.UsageInfo) AppCacheUsage {
	var u AppCacheUsage
	for _, r := range recs {
		u.Bytes += r.Size
		u.Records++
		u.InUse = u.InUse || r.InUse
		if r.LastUsedAt != nil && r.LastUsedAt.After(u.LastUsedAt) {
			u.LastUsedAt = *r.LastUsedAt
		}
	}
	return u
}

// AppCacheUsage reports the cache mounts of cacheID.
func (c *Client) AppCacheUsage(ctx context.Context, cacheID string, siblings []string) (AppCacheUsage, error) {
	if c == nil || c.bk == nil {
		return AppCacheUsage{}, ErrAppCacheNoBuilder
	}
	du, err := c.bk.DiskUsage(ctx)
	if err != nil {
		return AppCacheUsage{}, fmt.Errorf("build: disk usage: %w", err)
	}
	return SummarizeAppCache(AppCacheRecords(du, cacheID, siblings)), nil
}

// PruneAppCache removes only the cache mounts of cacheID, matched by record id
// so the shared cache and other apps are never touched.
func (c *Client) PruneAppCache(ctx context.Context, cacheID string, siblings []string) (AppCachePruneResult, error) {
	if c == nil || c.bk == nil {
		return AppCachePruneResult{}, ErrAppCacheNoBuilder
	}
	du, err := c.bk.DiskUsage(ctx)
	if err != nil {
		return AppCachePruneResult{}, fmt.Errorf("build: disk usage: %w", err)
	}
	recs := AppCacheRecords(du, cacheID, siblings)
	if len(recs) == 0 {
		return AppCachePruneResult{}, nil
	}
	filters := make([]string, 0, len(recs))
	for _, r := range recs {
		if r.InUse {
			return AppCachePruneResult{}, ErrAppCacheInUse
		}
		filters = append(filters, "id=="+r.ID)
	}
	ch := make(chan bkclient.UsageInfo)
	var res AppCachePruneResult
	done := make(chan struct{})
	go func() {
		defer close(done)
		for u := range ch {
			res.FreedBytes += u.Size
			res.Removed++
		}
	}()
	err = c.bk.Prune(ctx, ch, bkclient.WithFilter(filters), bkclient.PruneAll)
	close(ch)
	<-done
	if err != nil {
		return res, fmt.Errorf("build: prune app cache: %w", err)
	}
	return res, nil
}
