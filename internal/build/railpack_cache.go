package build

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// EnvBuildCachePerApp seeds the per app build cache switch; unset means on.
const EnvBuildCachePerApp = "APP_BUILD_CACHE_PER_APP"

// KeyBuildCachePerApp is the runtime setting key.
const KeyBuildCachePerApp = "build.cache_per_app"

const maxCacheIDLen = 48

var cachePerAppResolver atomic.Pointer[func(context.Context) (bool, error)]

// SetCachePerAppResolver wires the saved setting in at boot so changes apply live.
func SetCachePerAppResolver(fn func(context.Context) (bool, error)) {
	if fn == nil {
		cachePerAppResolver.Store(nil)
		return
	}
	cachePerAppResolver.Store(&fn)
}

func perAppCacheEnabled(ctx context.Context) bool {
	if fn := cachePerAppResolver.Load(); fn != nil {
		if on, err := (*fn)(ctx); err == nil {
			return on
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvBuildCachePerApp))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// SanitizeCacheID keeps [a-z0-9-] so the id is a safe BuildKit cache key part.
// With no explicit id it falls back to the repository part of tag.
func SanitizeCacheID(id, tag string) string {
	if strings.TrimSpace(id) == "" {
		id = tag
		if i := strings.LastIndex(id, ":"); i > strings.LastIndex(id, "/") {
			id = id[:i]
		}
		if i := strings.LastIndex(id, "/"); i >= 0 {
			id = id[i+1:]
		}
	}
	// FitCacheID hashes long names so apps sharing a prefix keep distinct ids.
	return FitCacheID(id)
}

// cacheLogLine reports the id and a best effort existence signal. BuildKit
// does not expose a cache mount by its id, so the signal is whether the
// builder holds any cache mount records at all.
func (c *Client) cacheLogLine(ctx context.Context, id string) string {
	if id == "" {
		return "build cache: no usable app id, using the shared cache"
	}
	existed := "unknown"
	if c.bk != nil {
		if du, err := c.bk.DiskUsage(ctx); err == nil {
			n := 0
			for _, r := range du {
				if r.RecordType == "exec.cachemount" {
					n++
				}
			}
			existed = fmt.Sprintf("builder holds %d cache mount records", n)
		}
	}
	return fmt.Sprintf("build cache: id=%s (%s)", id, existed)
}
