package api

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Env knob for how long a Pages domain diagnosis is served without a re-run.
const (
	envPagesDoctorCacheTTL     = "APP_PAGES_DOCTOR_CACHE_TTL"
	defaultPagesDoctorCacheTTL = 30 * time.Second
)

type siteDiagEntry struct {
	rows []siteDiagnosis
	at   time.Time
}

// siteDiagnosisCache keeps the last doctor result per site and domain so a
// polling dashboard never stacks doctor runs; one run is shared per key.
type siteDiagnosisCache struct {
	mu      sync.Mutex
	entries map[string]siteDiagEntry
	group   singleflight.Group
}

// get returns the rows for a key. force always re-runs; otherwise a fresh
// entry is returned as is, a stale one is returned while a background run
// refreshes it, and a missing one is run inline.
func (c *siteDiagnosisCache) get(ctx context.Context, key string, force bool, ttl time.Duration, run func(context.Context) []siteDiagnosis) []siteDiagnosis {
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && !force {
		if time.Since(e.at) >= ttl {
			bg := context.WithoutCancel(ctx)
			go c.load(bg, key, run)
		}
		return e.rows
	}
	return c.load(ctx, key, run)
}

func (c *siteDiagnosisCache) load(ctx context.Context, key string, run func(context.Context) []siteDiagnosis) []siteDiagnosis {
	v, _, _ := c.group.Do(key, func() (any, error) {
		rows := run(ctx)
		c.mu.Lock()
		if c.entries == nil {
			c.entries = map[string]siteDiagEntry{}
		}
		c.entries[key] = siteDiagEntry{rows: rows, at: time.Now()}
		c.mu.Unlock()
		return rows, nil
	})
	rows, _ := v.([]siteDiagnosis)
	return rows
}
