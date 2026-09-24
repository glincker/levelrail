package api

import (
	"context"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// defaultDiskUsageCacheTTL bounds how stale docker_disk_usage on
// GET /system/status can be. The underlying docker system df call takes
// hundreds of milliseconds on a host with real images and volumes.
const defaultDiskUsageCacheTTL = 30 * time.Second

// cachedDiskUsager memoises successful DiskUsage results for ttl. Errors
// are never cached, so a daemon hiccup does not stick.
type cachedDiskUsager struct {
	inner DockerDiskUsager
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	usage   docker.DiskUsage
	fetched time.Time
	valid   bool
}

func newCachedDiskUsager(inner DockerDiskUsager, ttl time.Duration) *cachedDiskUsager {
	return &cachedDiskUsager{inner: inner, ttl: ttl, now: time.Now}
}

// DiskUsage returns the cached result while it is fresh, else refetches.
// The lock is held across the fetch so concurrent callers share one call.
func (c *cachedDiskUsager) DiskUsage(ctx context.Context) (docker.DiskUsage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.valid && c.now().Sub(c.fetched) < c.ttl {
		return c.usage, nil
	}
	u, err := c.inner.DiskUsage(ctx)
	if err != nil {
		return docker.DiskUsage{}, err
	}
	c.usage, c.fetched, c.valid = u, c.now(), true
	return u, nil
}
