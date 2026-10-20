package repolist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envCacheTTL      = "APP_GIT_REPO_CACHE_TTL"
	envCacheStaleMax = "APP_GIT_REPO_CACHE_STALE_MAX"
	envRefreshMin    = "APP_GIT_REPO_REFRESH_MIN"
	envListMax       = "APP_GIT_REPO_LIST_MAX"

	defaultCacheTTL      = 5 * time.Minute
	defaultCacheStaleMax = 24 * time.Hour
	defaultRefreshMin    = 10 * time.Second
	defaultListMax       = 10000
	fetchTimeout         = 3 * time.Minute
)

// Config holds the cache bounds, all overridable through APP_GIT_REPO_*.
type Config struct {
	// TTL is how long a snapshot is served without any provider call.
	TTL time.Duration
	// StaleMax is how long an expired snapshot may still be served while it
	// refreshes in the background or the provider is failing.
	StaleMax time.Duration
	// RefreshMin throttles manual refreshes of one connection.
	RefreshMin time.Duration
	// MaxItems caps one connection's list; beyond it the list is marked truncated.
	MaxItems int
}

// ConfigFromEnv reads the cache bounds from the environment.
func ConfigFromEnv() Config {
	c := Config{TTL: defaultCacheTTL, StaleMax: defaultCacheStaleMax, RefreshMin: defaultRefreshMin, MaxItems: defaultListMax}
	if d, err := time.ParseDuration(os.Getenv(envCacheTTL)); err == nil && d >= 0 {
		c.TTL = d
	}
	if d, err := time.ParseDuration(os.Getenv(envCacheStaleMax)); err == nil && d >= 0 {
		c.StaleMax = d
	}
	if d, err := time.ParseDuration(os.Getenv(envRefreshMin)); err == nil && d >= 0 {
		c.RefreshMin = d
	}
	if n, ok := envInt(envListMax); ok && n > 0 {
		c.MaxItems = n
	}
	return c
}

func envInt(name string) (int, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// Snapshot is one connection's full repository list as last fetched.
type Snapshot struct {
	Repos     []Repo
	Truncated bool
	FetchedAt time.Time
	// Meta is provider-specific revalidation data (GitHub page ETags).
	Meta string
}

// FetchFunc lists a connection's repositories. prior is the previous
// snapshot (possibly expired) for conditional requests, nil when none.
type FetchFunc func(ctx context.Context, prior *Snapshot) (Snapshot, error)

// Result is what a Get returns: the snapshot plus how fresh it is.
type Result struct {
	Snapshot
	// Stale is true when the snapshot is older than the TTL.
	Stale bool
	// Refreshing is true when a background refresh was started for it.
	Refreshing bool
	// RateLimit is set when a refresh was blocked by the provider's rate limit.
	RateLimit *gitprovider.RateLimitError
	// RefreshErr is set when a refresh failed and the stale snapshot was served instead.
	RefreshErr error
}

// Store persists snapshots; *store.DB satisfies it.
type Store interface {
	GetGitRepoListCache(ctx context.Context, key string) (store.GitRepoListCacheEntry, error)
	PutGitRepoListCache(ctx context.Context, e store.GitRepoListCacheEntry) error
	DeleteGitRepoListCachePrefix(ctx context.Context, prefix string) error
}

// Cache serves repository lists per connection key: fresh from the store,
// stale-while-revalidate, with concurrent fetches for one key sharing a
// single provider call.
type Cache struct {
	store  Store
	cfg    Config
	logger *slog.Logger
	now    func() time.Time
	sf     singleflight.Group
	wg     sync.WaitGroup

	mu     sync.Mutex
	epochs map[string]uint64
}

// NewCache builds a Cache over st.
func NewCache(st Store, cfg Config, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cache{store: st, cfg: cfg, logger: logger, now: time.Now, epochs: map[string]uint64{}}
}

// Config returns the cache bounds in effect.
func (c *Cache) Config() Config { return c.cfg }

// Wait blocks until background refreshes finish (tests, shutdown).
func (c *Cache) Wait() { c.wg.Wait() }

type payload struct {
	Repos []Repo `json:"repos"`
}

func (c *Cache) load(ctx context.Context, key string) (*Snapshot, error) {
	e, err := c.store.GetGitRepoListCache(ctx, key)
	if errors.Is(err, store.ErrGitRepoListCacheMiss) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load repo list cache: %w", err)
	}
	var p payload
	if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
		c.logger.Warn("repolist: dropping unreadable cache entry", slog.String("cache_key", key), slog.String("error", err.Error()))
		return nil, nil
	}
	return &Snapshot{Repos: p.Repos, Truncated: e.Truncated, FetchedAt: e.FetchedAt, Meta: e.Meta}, nil
}

func (c *Cache) save(ctx context.Context, key, provider string, s Snapshot) error {
	raw, err := json.Marshal(payload{Repos: s.Repos})
	if err != nil {
		return fmt.Errorf("encode repo list cache: %w", err)
	}
	if err := c.store.PutGitRepoListCache(ctx, store.GitRepoListCacheEntry{
		Key: key, Provider: provider, Payload: string(raw), Meta: s.Meta, Truncated: s.Truncated, FetchedAt: s.FetchedAt,
	}); err != nil {
		return fmt.Errorf("save repo list cache: %w", err)
	}
	return nil
}

// Get returns the repository list for key, fetching through fetch when the
// cache is empty, expired beyond StaleMax, or a manual refresh is asked for.
func (c *Cache) Get(ctx context.Context, key, provider string, refresh bool, fetch FetchFunc) (Result, error) {
	prior, err := c.load(ctx, key)
	if err != nil {
		return Result{}, err
	}
	now := c.now()
	if prior != nil {
		age := now.Sub(prior.FetchedAt)
		switch {
		case refresh && age >= c.cfg.RefreshMin:
		case age < c.cfg.TTL || (refresh && age < c.cfg.RefreshMin):
			return Result{Snapshot: *prior}, nil
		case age < c.cfg.StaleMax:
			c.refreshInBackground(key, provider, prior, fetch)
			return Result{Snapshot: *prior, Stale: true, Refreshing: true}, nil
		}
	}

	snap, err := c.fetchShared(ctx, key, provider, prior, fetch)
	if err == nil {
		return Result{Snapshot: snap}, nil
	}
	if prior != nil && now.Sub(prior.FetchedAt) < c.cfg.StaleMax {
		res := Result{Snapshot: *prior, Stale: true, RefreshErr: err}
		if rl, ok := gitprovider.AsRateLimit(err); ok {
			res.RateLimit = rl
		}
		return res, nil
	}
	return Result{}, err
}

// generationOf sums the invalidations that cover key, so a fetch that
// started before an Invalidate can tell its result is already obsolete.
func (c *Cache) generationOf(key string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sum uint64
	for prefix, n := range c.epochs {
		if strings.HasPrefix(key, prefix) {
			sum += n
		}
	}
	return sum
}

// fetchShared runs one provider fetch per key at a time; every concurrent
// caller waits on the same call. The call outlives a cancelled first caller.
func (c *Cache) fetchShared(ctx context.Context, key, provider string, prior *Snapshot, fetch FetchFunc) (Snapshot, error) {
	ch := c.sf.DoChan(key, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		gen := c.generationOf(key)
		snap, err := fetch(fctx, prior)
		if err != nil {
			return nil, err
		}
		snap.FetchedAt = c.now()
		if len(snap.Repos) > c.cfg.MaxItems {
			snap.Repos = snap.Repos[:c.cfg.MaxItems]
			snap.Truncated = true
		}
		if c.generationOf(key) != gen {
			return snap, nil
		}
		if err := c.save(fctx, key, provider, snap); err != nil {
			c.logger.Warn("repolist: could not persist repo list", slog.String("cache_key", key), slog.String("error", err.Error()))
		}
		return snap, nil
	})
	select {
	case <-ctx.Done():
		return Snapshot{}, fmt.Errorf("wait for repo list: %w", ctx.Err())
	case r := <-ch:
		if r.Err != nil {
			return Snapshot{}, fmt.Errorf("fetch repo list: %w", r.Err)
		}
		snap, ok := r.Val.(Snapshot)
		if !ok {
			return Snapshot{}, errors.New("fetch repo list: unexpected result type")
		}
		return snap, nil
	}
}

func (c *Cache) refreshInBackground(key, provider string, prior *Snapshot, fetch FetchFunc) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if _, err := c.fetchShared(context.Background(), key, provider, prior, fetch); err != nil {
			c.logger.Warn("repolist: background refresh failed", slog.String("cache_key", key), slog.String("error", err.Error()))
		}
	}()
}

// Invalidate drops every cached list whose key starts with prefix, and
// discards any fetch still in flight for them so it cannot re-save old data.
func (c *Cache) Invalidate(ctx context.Context, prefix string) error {
	c.mu.Lock()
	c.epochs[prefix]++
	c.mu.Unlock()
	if err := c.store.DeleteGitRepoListCachePrefix(ctx, prefix); err != nil {
		return fmt.Errorf("invalidate repo list cache %q: %w", prefix, err)
	}
	return nil
}
