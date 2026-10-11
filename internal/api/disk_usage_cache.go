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
// are never cached, so a daemon hiccup does not stick. With stale > 0, an
// expired entry younger than ttl+stale is served while one background
// refresh runs, so a request never pays the docker call after warm-up.
type cachedDiskUsager struct {
	inner DockerDiskUsager
	ttl   time.Duration
	stale time.Duration
	now   func() time.Time

	mu      sync.Mutex
	usage   docker.DiskUsage
	fetched time.Time
	valid   bool
	flight  chan struct{} // non-nil while a background refresh runs
}

func newCachedDiskUsager(inner DockerDiskUsager, ttl time.Duration) *cachedDiskUsager {
	return &cachedDiskUsager{inner: inner, ttl: ttl, now: time.Now}
}

// DiskUsage returns the cached result while it is fresh, else refetches.
// A blocking fetch holds the lock so concurrent callers share one call.
func (c *cachedDiskUsager) DiskUsage(ctx context.Context) (docker.DiskUsage, error) {
	c.mu.Lock()
	if c.valid {
		age := c.now().Sub(c.fetched)
		if age < c.ttl {
			defer c.mu.Unlock()
			return c.usage, nil
		}
		if c.stale > 0 && age < c.ttl+c.stale {
			c.startRefreshLocked()
			defer c.mu.Unlock()
			return c.usage, nil
		}
	}
	if flight := c.flight; flight != nil {
		c.mu.Unlock()
		select {
		case <-flight:
		case <-ctx.Done():
			return docker.DiskUsage{}, ctx.Err()
		}
		c.mu.Lock()
		if c.valid {
			defer c.mu.Unlock()
			return c.usage, nil
		}
	}
	defer c.mu.Unlock()
	u, err := c.inner.DiskUsage(ctx)
	if err != nil {
		return docker.DiskUsage{}, err
	}
	c.usage, c.fetched, c.valid = u, c.now(), true
	return u, nil
}

func (c *cachedDiskUsager) startRefreshLocked() {
	if c.flight != nil {
		return
	}
	c.flight = make(chan struct{})
	go c.refresh(c.flight)
}

// refresh fetches outside the request path; a failure keeps the old value.
func (c *cachedDiskUsager) refresh(flight chan struct{}) {
	u, err := c.inner.DiskUsage(context.Background())
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.usage, c.fetched, c.valid = u, c.now(), true
	}
	c.flight = nil
	close(flight)
}

// prewarm starts one background fetch so the first dashboard load after a
// restart does not wait on the docker call.
func (c *cachedDiskUsager) prewarm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.valid {
		c.startRefreshLocked()
	}
}
