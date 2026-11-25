package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCache() *viewerCache {
	return &viewerCache{entries: map[string]viewerCacheEntry{}, gen: map[string]uint64{}}
}

func TestViewerCacheReusesWithinTTLAndDisablesAtZero(t *testing.T) {
	t.Parallel()
	c := newTestCache()
	var n atomic.Int32
	load := func() (any, error) { return int(n.Add(1)), nil }
	v1, _, hit1, _ := c.do(time.Minute, "d", "k", load)
	v2, _, hit2, _ := c.do(time.Minute, "d", "k", load)
	if hit1 || !hit2 || v1 != v2 || n.Load() != 1 {
		t.Fatalf("want one load then a hit, got hit1=%v hit2=%v loads=%d", hit1, hit2, n.Load())
	}
	_, _, _, _ = c.do(0, "d", "k", load)
	_, _, _, _ = c.do(0, "d", "k", load)
	if n.Load() != 3 {
		t.Fatalf("ttl 0 must always load, loads=%d", n.Load())
	}
}

func TestViewerCacheInvalidateDropsAndBlocksInFlightStore(t *testing.T) {
	t.Parallel()
	c := newTestCache()
	var n atomic.Int32
	load := func() (any, error) { return int(n.Add(1)), nil }
	_, _, _, _ = c.do(time.Minute, "d", "k", load)
	c.invalidate("d")
	if _, _, hit, _ := c.do(time.Minute, "d", "k", load); hit {
		t.Fatal("entry must be gone after invalidate")
	}
	_, _, _, _ = c.do(time.Minute, "d", "slow", func() (any, error) {
		c.invalidate("d")
		return 1, nil
	})
	if _, _, hit, _ := c.do(time.Minute, "d", "slow", load); hit {
		t.Fatal("a load that raced an invalidate must not be stored")
	}
}

func TestViewerCacheSingleflightCollapsesConcurrentLoads(t *testing.T) {
	t.Parallel()
	c := newTestCache()
	var n atomic.Int32
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, _ = c.do(time.Minute, "d", "k", func() (any, error) {
				n.Add(1)
				<-gate
				return 1, nil
			})
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("loads = %d, want 1", n.Load())
	}
}

func TestViewerCacheFailureIsNotCachedAndFollowersRetry(t *testing.T) {
	t.Parallel()
	c := newTestCache()
	_, _, _, err := c.do(time.Minute, "d", "k", func() (any, error) { return nil, errViewerLoadFailed })
	if !errors.Is(err, errViewerLoadFailed) {
		t.Fatalf("err = %v", err)
	}
	if v, _, hit, err := c.do(time.Minute, "d", "k", func() (any, error) { return 7, nil }); err != nil || hit || v != 7 {
		t.Fatalf("after a failure the next call must load: v=%v hit=%v err=%v", v, hit, err)
	}
}
