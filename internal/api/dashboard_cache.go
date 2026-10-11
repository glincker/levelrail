package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// dashboardCache is a stale-while-revalidate cache with singleflight loads.
// Values must not depend on the caller: handlers cache raw telemetry or
// probe results and apply the viewer's visibility filter after the read.
type dashboardCache struct {
	ttl   time.Duration
	stale time.Duration
	now   func() time.Time

	mu      sync.Mutex
	entries map[string]dashboardEntry
	group   singleflight.Group
}

type dashboardEntry struct {
	value   any
	fetched time.Time
}

const dashboardCacheMaxEntries = 256

func newDashboardCache(ttl, stale time.Duration) *dashboardCache {
	return &dashboardCache{ttl: ttl, stale: stale, now: time.Now, entries: map[string]dashboardEntry{}}
}

func (c *dashboardCache) load(key string, fn func(ctx context.Context) (any, error)) (any, error) {
	v, err, _ := c.group.Do(key, func() (any, error) { return c.loadInner(key, fn) })
	return v, err
}

func (c *dashboardCache) evictLocked() {
	cutoff := c.now().Add(-(c.ttl + c.stale))
	for k, e := range c.entries {
		if e.fetched.Before(cutoff) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) >= dashboardCacheMaxEntries {
		c.entries = map[string]dashboardEntry{}
	}
}

// get returns the cached value for key, loading it with fn when missing or
// too old. fn runs detached from the caller's context so one cancelled
// request cannot fail the load other requests share.
func (c *dashboardCache) get(ctx context.Context, key string, fn func(ctx context.Context) (any, error)) (any, error) {
	if c.ttl > 0 {
		c.mu.Lock()
		e, ok := c.entries[key]
		c.mu.Unlock()
		if ok {
			age := c.now().Sub(e.fetched)
			if age < c.ttl {
				return e.value, nil
			}
			if age < c.ttl+c.stale {
				go func() { _, _ = c.load(key, fn) }()
				return e.value, nil
			}
		}
	}
	ch := c.group.DoChan(key, func() (any, error) { return c.loadInner(key, fn) })
	select {
	case res := <-ch:
		return res.Val, res.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *dashboardCache) loadInner(key string, fn func(ctx context.Context) (any, error)) (v any, err error) {
	// Loads run on their own goroutine, outside net/http's per request recover.
	defer func() {
		if r := recover(); r != nil {
			v, err = nil, fmt.Errorf("api: dashboard load %s panicked: %v", key, r)
		}
	}()
	val, err := fn(context.Background())
	if err != nil {
		return nil, err
	}
	if c.ttl > 0 {
		c.mu.Lock()
		if len(c.entries) >= dashboardCacheMaxEntries {
			c.evictLocked()
		}
		c.entries[key] = dashboardEntry{value: val, fetched: c.now()}
		c.mu.Unlock()
	}
	return val, nil
}

// WithDashboardCache enables the short TTL cache behind the dashboard's
// aggregate endpoints and the docker disk usage warm-up. ttl <= 0 keeps
// every request live; the binary resolves APP_DASHBOARD_CACHE_TTL and _STALE.
func WithDashboardCache(ttl, stale time.Duration) Option {
	return func(rt *Router) {
		rt.dash = newDashboardCache(ttl, stale)
		rt.dashPrewarm = ttl > 0
	}
}

func (rt *Router) warmDashboardCaches() {
	if !rt.dashPrewarm {
		return
	}
	if c, ok := rt.dockerDiskUsage.(*cachedDiskUsager); ok {
		c.stale = rt.dash.stale
		c.prewarm()
	}
}

// dockerPingTimeout bounds the detached ping a cached status load makes.
const dockerPingTimeout = 10 * time.Second

type dockerPingResult struct{ err error }

// pingDocker is dockerPinger.Ping behind the dashboard cache, shared by
// system/status and system/doctor so one dashboard load pings once.
func (rt *Router) pingDocker(ctx context.Context) error {
	defer timePhase(ctx, "docker_ping")()
	if rt.dash.ttl <= 0 {
		return rt.dockerPinger.Ping(ctx)
	}
	v, err := rt.dash.get(ctx, "docker_ping", func(ctx context.Context) (any, error) {
		pingCtx, cancel := context.WithTimeout(ctx, dockerPingTimeout)
		defer cancel()
		return dockerPingResult{err: rt.dockerPinger.Ping(pingCtx)}, nil
	})
	if err != nil {
		return err
	}
	return v.(dockerPingResult).err
}

// latestByMetric is telemetry.LatestByMetric behind the dashboard cache.
// The returned slice is shared between callers and must not be modified.
func (rt *Router) latestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error) {
	defer timePhase(ctx, "telemetry_latest")()
	if rt.dash.ttl <= 0 {
		return rt.telemetry.LatestByMetric(ctx, metric)
	}
	v, err := rt.dash.get(ctx, "latest:"+metric, func(ctx context.Context) (any, error) {
		return rt.telemetry.LatestByMetric(ctx, metric)
	})
	if err != nil {
		return nil, err
	}
	samples, _ := v.([]telemetry.Sample)
	return samples, nil
}
