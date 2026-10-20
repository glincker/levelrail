package repolist

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestCache(t *testing.T) (*Cache, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	c := NewCache(NewMemoryStore(), Config{TTL: time.Minute, StaleMax: time.Hour, RefreshMin: 10 * time.Second, MaxItems: 5}, nil)
	c.now = clk.now
	return c, clk
}

func counting(repos ...Repo) (FetchFunc, *atomic.Int32) {
	var n atomic.Int32
	return func(context.Context, *Snapshot) (Snapshot, error) {
		n.Add(1)
		return Snapshot{Repos: repos}, nil
	}, &n
}

func TestCacheServesFreshWithoutFetching(t *testing.T) {
	c, clk := newTestCache(t)
	fetch, calls := counting(Repo{FullName: "a/b"})
	for range 3 {
		res, err := c.Get(t.Context(), "github:1", "github", false, fetch)
		if err != nil || len(res.Repos) != 1 || res.Stale {
			t.Fatalf("Get() = %+v, %v", res, err)
		}
		clk.advance(10 * time.Second)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetches = %d, want 1 within the TTL", calls.Load())
	}
}

func TestCacheExpiredEntryRefreshesInBackground(t *testing.T) {
	c, clk := newTestCache(t)
	fetch, calls := counting(Repo{FullName: "a/b"})
	if _, err := c.Get(t.Context(), "k", "github", false, fetch); err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Minute)
	res, err := c.Get(t.Context(), "k", "github", false, fetch)
	if err != nil || !res.Stale || !res.Refreshing {
		t.Fatalf("Get() = %+v, %v, want the stale list served while refreshing", res, err)
	}
	c.Wait()
	if calls.Load() != 2 {
		t.Fatalf("fetches = %d, want 2 after the background refresh", calls.Load())
	}
	res, _ = c.Get(t.Context(), "k", "github", false, fetch)
	if res.Stale {
		t.Fatal("list still stale after the background refresh")
	}
}

func TestCacheSingleFlightSharesOneFetch(t *testing.T) {
	c, _ := newTestCache(t)
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context, _ *Snapshot) (Snapshot, error) {
		calls.Add(1)
		<-release
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Repos: []Repo{{FullName: "a/b"}}}, nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := c.Get(t.Context(), "k", "github", false, fetch); err != nil || len(res.Repos) != 1 {
				t.Errorf("Get() = %+v, %v", res, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 shared by 8 callers", calls.Load())
	}
}

func TestCacheCancelledCallerDoesNotAbortSharedFetch(t *testing.T) {
	c, _ := newTestCache(t)
	release := make(chan struct{})
	fetch := func(ctx context.Context, _ *Snapshot) (Snapshot, error) {
		<-release
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Repos: []Repo{{FullName: "a/b"}}}, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() { _, err := c.Get(ctx, "k", "github", false, fetch); errc <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller err = %v", err)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		res, err := c.Get(t.Context(), "k", "github", false, fetch)
		if err == nil && len(res.Repos) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fetch result never landed in the cache: %+v %v", res, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCacheInvalidateDropsEntriesAndInFlightResults(t *testing.T) {
	c, _ := newTestCache(t)
	fetch, calls := counting(Repo{FullName: "a/b"})
	for _, k := range []string{"gitlab", "github:1", "github:2"} {
		if _, err := c.Get(t.Context(), k, "x", false, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Invalidate(t.Context(), "github"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"github:1", "github:2", "gitlab"} {
		if _, err := c.Get(t.Context(), k, "x", false, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("fetches = %d, want 5 (3 initial + 2 github refetches, gitlab untouched)", calls.Load())
	}

	release := make(chan struct{})
	slow := func(ctx context.Context, _ *Snapshot) (Snapshot, error) {
		<-release
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Repos: []Repo{{FullName: "old/token"}}}, nil
	}
	done := make(chan struct{})
	go func() { _, _ = c.Get(t.Context(), "bitbucket", "bitbucket", false, slow); close(done) }()
	time.Sleep(20 * time.Millisecond)
	if err := c.Invalidate(t.Context(), "bitbucket"); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	if s, _ := c.load(t.Context(), "bitbucket"); s != nil {
		t.Fatal("a fetch that started before the invalidation re-saved the old list")
	}
}

func TestCacheRateLimitedRefreshServesStale(t *testing.T) {
	c, clk := newTestCache(t)
	fetch, _ := counting(Repo{FullName: "a/b"})
	if _, err := c.Get(t.Context(), "k", "github", false, fetch); err != nil {
		t.Fatal(err)
	}
	clk.advance(3 * time.Hour)
	limited := func(context.Context, *Snapshot) (Snapshot, error) {
		return Snapshot{}, &gitprovider.RateLimitError{Provider: "github", NextCheck: clk.now().Add(time.Minute)}
	}
	if _, err := c.Get(t.Context(), "k", "github", false, limited); err == nil {
		t.Fatalf("a list older than StaleMax must not be served, err = %v", err)
	}
	clk.advance(-3*time.Hour + 2*time.Minute)
	res, err := c.Get(t.Context(), "k", "github", true, limited)
	if err != nil || !res.Stale || res.RateLimit == nil || len(res.Repos) != 1 {
		t.Fatalf("Get() = %+v, %v, want the stale list with a rate limit notice", res, err)
	}
}

func TestCacheRateLimitWithNothingCachedIsTyped(t *testing.T) {
	c, clk := newTestCache(t)
	next := clk.now().Add(90 * time.Second)
	_, err := c.Get(t.Context(), "k", "github", false, func(context.Context, *Snapshot) (Snapshot, error) {
		return Snapshot{}, &gitprovider.RateLimitError{Provider: "github", NextCheck: next}
	})
	rl, ok := gitprovider.AsRateLimit(err)
	if !ok || !rl.NextCheck.Equal(next) {
		t.Fatalf("err = %v, want a RateLimitError carrying the next check time", err)
	}
}

func TestCacheManualRefreshIsThrottled(t *testing.T) {
	c, clk := newTestCache(t)
	fetch, calls := counting(Repo{FullName: "a/b"})
	_, _ = c.Get(t.Context(), "k", "github", false, fetch)
	_, _ = c.Get(t.Context(), "k", "github", true, fetch)
	if calls.Load() != 1 {
		t.Fatalf("fetches = %d, want a refresh within RefreshMin to be served from cache", calls.Load())
	}
	clk.advance(15 * time.Second)
	_, _ = c.Get(t.Context(), "k", "github", true, fetch)
	if calls.Load() != 2 {
		t.Fatalf("fetches = %d, want the refresh to hit the provider after RefreshMin", calls.Load())
	}
}

func TestCacheCapsAndMarksTruncated(t *testing.T) {
	c, _ := newTestCache(t)
	repos := make([]Repo, 9)
	fetch, _ := counting(repos...)
	res, err := c.Get(t.Context(), "k", "github", false, fetch)
	if err != nil || len(res.Repos) != 5 || !res.Truncated {
		t.Fatalf("Get() = %d repos truncated=%v err=%v, want 5 truncated", len(res.Repos), res.Truncated, err)
	}
}

func TestCachePassesPriorSnapshotToFetch(t *testing.T) {
	c, clk := newTestCache(t)
	_, _ = c.Get(t.Context(), "k", "github", false, func(context.Context, *Snapshot) (Snapshot, error) {
		return Snapshot{Repos: []Repo{{FullName: "a/b"}}, Meta: `["etag"]`}, nil
	})
	clk.advance(2 * time.Minute)
	var got *Snapshot
	_, _ = c.Get(t.Context(), "k", "github", true, func(_ context.Context, prior *Snapshot) (Snapshot, error) {
		got = prior
		return Snapshot{}, nil
	})
	c.Wait()
	if got == nil || got.Meta != `["etag"]` {
		t.Fatalf("prior = %+v, want the previous snapshot with its etag meta", got)
	}
}
