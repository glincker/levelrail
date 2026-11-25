package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// viewerCacheMaxEntries bounds memory; entries are small listings and expire fast.
const viewerCacheMaxEntries = 256

// errViewerLoadFailed means the loader already wrote its own error response.
var errViewerLoadFailed = errors.New("api: viewer load failed")

type viewerCacheEntry struct {
	val any
	at  time.Time
	db  string
}

// viewerCache reuses read-only overviews for a short, live-tunable TTL and
// collapses concurrent identical loads into one container exec.
type viewerCache struct {
	mu      sync.Mutex
	entries map[string]viewerCacheEntry
	gen     map[string]uint64
	flight  singleflight.Group
}

var viewerOverviews = &viewerCache{entries: map[string]viewerCacheEntry{}, gen: map[string]uint64{}}

// viewerCacheTTL reads the live setting; 0 disables caching.
func (rt *Router) viewerCacheTTL(ctx context.Context) time.Duration {
	return time.Duration(max(0, rt.runtimeSettings.Int(ctx, runtimesettings.KeyDBViewerCacheSeconds))) * time.Second
}

// do returns the cached value for key or runs load once for all callers.
// A loader that fails must write its own response and return errViewerLoadFailed.
func (c *viewerCache) do(ttl time.Duration, db, key string, load func() (any, error)) (val any, age time.Duration, hit bool, err error) {
	if ttl <= 0 {
		v, err := load()
		return v, 0, false, err
	}
	if v, a, ok := c.get(key, ttl); ok {
		return v, a, true, nil
	}
	ran := false
	v, err, _ := c.flight.Do(key, func() (any, error) {
		ran = true
		c.mu.Lock()
		g := c.gen[db]
		c.mu.Unlock()
		v, err := load()
		if err == nil {
			c.put(key, db, g, v)
		}
		return v, err
	})
	if err != nil && !ran {
		// The leader failed or was cancelled, so this caller loads for itself.
		v, err = load()
	}
	return v, 0, false, err
}

func (c *viewerCache) get(key string, ttl time.Duration) (any, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, 0, false
	}
	age := time.Since(e.at)
	if age >= ttl {
		delete(c.entries, key)
		return nil, 0, false
	}
	return e.val, age, true
}

// put skips the store when the database was invalidated while loading.
func (c *viewerCache) put(key, db string, gen uint64, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen[db] != gen {
		return
	}
	if len(c.entries) >= viewerCacheMaxEntries {
		clear(c.entries)
	}
	c.entries[key] = viewerCacheEntry{val: v, at: time.Now(), db: db}
}

// invalidate drops every entry of one database, for writes and state changes.
func (c *viewerCache) invalidate(db string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen[db]++
	for k, e := range c.entries {
		if e.db == db {
			delete(c.entries, k)
		}
	}
}
