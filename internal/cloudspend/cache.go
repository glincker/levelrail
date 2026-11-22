package cloudspend

import (
	"context"
	"sync"
	"time"
)

// Key identifies one cached report. Rate limits use AccountID alone so a
// caller cannot dodge them by varying Scope.
type Key struct {
	AccountID string
	Scope     string
}

// Fetch loads a fresh report from the provider.
type Fetch func(ctx context.Context) (SpendReport, error)

// Decision is the outcome of a forced refresh. The API turns a denial into
// 429 with Retry-After set to RetryAfter.
type Decision struct {
	Allowed    bool
	Reason     string
	RetryAfter time.Duration
}

// Refusal reasons.
const (
	ReasonAccountInterval = "account_interval"
	ReasonUserHourly      = "user_hourly"
)

type flight struct {
	done chan struct{}
	rep  SpendReport
	err  error
}

type entry struct {
	rep     SpendReport
	errCode string
}

// Cache serves reports from memory, runs one fetch at a time per key and
// rate limits forced refreshes. A failed fetch keeps the last good report.
type Cache struct {
	cfg func() Config

	mu          sync.Mutex
	entries     map[Key]*entry
	flights     map[Key]*flight
	lastRefresh map[string]time.Time
	userHits    map[string][]time.Time
}

// NewCache builds a cache that reads its limits live from cfg.
func NewCache(cfg func() Config) *Cache {
	return &Cache{
		cfg: cfg, entries: map[Key]*entry{}, flights: map[Key]*flight{},
		lastRefresh: map[string]time.Time{}, userHits: map[string][]time.Time{},
	}
}

// Get returns a fresh entry, or a stale one while one background fetch runs,
// or waits for the first fetch when nothing is cached.
func (c *Cache) Get(ctx context.Context, k Key, fetch Fetch) (SpendReport, error) {
	cfg := c.cfg()
	now := cfg.now()
	c.mu.Lock()
	e := c.entries[k]
	if e != nil && now.Sub(e.rep.FetchedAt) < cfg.CacheTTL {
		rep := c.view(e, cfg, now)
		c.mu.Unlock()
		return rep, nil
	}
	if e != nil {
		c.startLocked(k, fetch)
		rep := c.view(e, cfg, now)
		rep.Stale = true
		c.mu.Unlock()
		return rep, nil
	}
	f := c.startLocked(k, fetch)
	c.mu.Unlock()
	return c.wait(ctx, f)
}

// Refresh is a forced refresh. It obeys the per-account interval and the
// per-user hourly limit; a denial returns the cached report, if any.
func (c *Cache) Refresh(ctx context.Context, k Key, user string, fetch Fetch) (SpendReport, Decision, error) {
	cfg := c.cfg()
	now := cfg.now()
	c.mu.Lock()
	if d := c.limitLocked(k.AccountID, user, cfg, now); !d.Allowed {
		var rep SpendReport
		if e := c.entries[k]; e != nil {
			rep = c.view(e, cfg, now)
		}
		c.mu.Unlock()
		return rep, d, nil
	}
	c.lastRefresh[k.AccountID] = now
	if user != "" {
		c.userHits[user] = append(c.userHits[user], now)
	}
	f := c.startLocked(k, fetch)
	c.mu.Unlock()
	rep, err := c.wait(ctx, f)
	return rep, Decision{Allowed: true}, err
}

func (c *Cache) limitLocked(account, user string, cfg Config, now time.Time) Decision {
	if last, ok := c.lastRefresh[account]; ok && cfg.RefreshMinInterval > 0 {
		if wait := cfg.RefreshMinInterval - now.Sub(last); wait > 0 {
			return Decision{Reason: ReasonAccountInterval, RetryAfter: wait}
		}
	}
	if user == "" || cfg.RefreshPerUserPerHour <= 0 {
		return Decision{Allowed: true}
	}
	recent := c.userHits[user][:0]
	for _, t := range c.userHits[user] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	c.userHits[user] = recent
	if len(recent) >= cfg.RefreshPerUserPerHour {
		return Decision{Reason: ReasonUserHourly, RetryAfter: time.Hour - now.Sub(recent[0])}
	}
	return Decision{Allowed: true}
}

// startLocked joins the running flight for k or starts one. The fetch runs
// detached so a caller that gives up does not cancel a shared fetch.
func (c *Cache) startLocked(k Key, fetch Fetch) *flight {
	if f := c.flights[k]; f != nil {
		return f
	}
	f := &flight{done: make(chan struct{})}
	c.flights[k] = f
	go func() {
		rep, err := fetch(context.Background())
		c.mu.Lock()
		defer c.mu.Unlock()
		now := c.cfg().now()
		switch {
		case err == nil:
			rep.FetchedAt = now
			c.entries[k] = &entry{rep: rep}
			f.rep = rep
		case c.entries[k] != nil:
			c.entries[k].errCode = ErrorCode(err)
			f.rep = c.view(c.entries[k], c.cfg(), now)
		}
		f.err = err
		delete(c.flights, k)
		close(f.done)
	}()
	return f
}

func (c *Cache) wait(ctx context.Context, f *flight) (SpendReport, error) {
	select {
	case <-f.done:
		if f.err != nil && f.rep.AccountID == "" {
			return SpendReport{}, f.err
		}
		return f.rep, nil
	case <-ctx.Done():
		return SpendReport{}, ctx.Err()
	}
}

// view copies an entry and sets the stale and error markers.
func (c *Cache) view(e *entry, cfg Config, now time.Time) SpendReport {
	rep := e.rep
	rep.ErrorCode = e.errCode
	if e.errCode != "" {
		rep.Stale = true
	}
	if cfg.StaleAfter > 0 && rep.DataAsOf != nil && now.Sub(*rep.DataAsOf) > cfg.StaleAfter {
		rep.Stale = true
	}
	return rep
}
