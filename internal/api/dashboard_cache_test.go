package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDashboardCache(t *testing.T) {
	var nowSec atomic.Int64
	nowSec.Store(1000)
	c := newDashboardCache(10*time.Second, 60*time.Second)
	c.now = func() time.Time { return time.Unix(nowSec.Load(), 0) }
	var calls atomic.Int32
	load := func(context.Context) (any, error) { return int(calls.Add(1)), nil } //nolint:unparam // loader signature
	ctx := context.Background()

	get := func(key string) int {
		v, err := c.get(ctx, key, load)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		return v.(int)
	}

	if got := get("k"); got != 1 {
		t.Fatalf("first = %d", got)
	}
	if got := get("k"); got != 1 || calls.Load() != 1 {
		t.Fatalf("fresh hit = %d calls %d", got, calls.Load())
	}
	if got := get("other"); got != 2 {
		t.Fatalf("distinct key shared an entry: %d", got)
	}

	nowSec.Add(30)
	if got := get("k"); got != 1 {
		t.Fatalf("stale window must serve the old value, got %d", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	got := 1
	for got == 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		got = get("k")
	}
	if got != 3 {
		t.Fatalf("after background refresh = %d, calls %d", got, calls.Load())
	}

	nowSec.Add(300)
	before := calls.Load()
	if got := get("k"); got != int(before)+1 {
		t.Fatalf("past stale window must reload synchronously, got %d", got)
	}
}

func TestDashboardCache_ErrorsNotCachedAndDisabledPassesThrough(t *testing.T) {
	c := newDashboardCache(10*time.Second, 0)
	var calls int
	_, err := c.get(context.Background(), "k", func(context.Context) (any, error) {
		calls++
		return nil, context.DeadlineExceeded
	})
	if err == nil {
		t.Fatal("want error")
	}
	v, err := c.get(context.Background(), "k", func(context.Context) (any, error) {
		calls++
		return "ok", nil
	})
	if err != nil || v != "ok" || calls != 2 {
		t.Fatalf("v=%v err=%v calls=%d", v, err, calls)
	}

	off := newDashboardCache(0, 0)
	n := 0
	for i := 0; i < 3; i++ {
		_, _ = off.get(context.Background(), "k", func(context.Context) (any, error) { n++; return n, nil })
	}
	if n != 3 {
		t.Fatalf("disabled cache loads = %d, want 3", n)
	}
}

func TestDashboardCache_ConcurrentMissesShareOneLoad(t *testing.T) {
	c := newDashboardCache(time.Minute, 0)
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.get(context.Background(), "k", func(context.Context) (any, error) {
				calls.Add(1)
				<-release
				return 1, nil
			})
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("loads = %d, want 1", calls.Load())
	}
}

func TestRequestTimingsBreakdown(t *testing.T) {
	rt := &requestTimings{parts: map[string]time.Duration{"fast": time.Millisecond, "slow": 900 * time.Millisecond}}
	got := rt.breakdown()
	if !strings.HasPrefix(got, "slow=900ms,") || !strings.Contains(got, "fast=1ms") {
		t.Errorf("breakdown = %q", got)
	}
	if (&requestTimings{}).breakdown() != "" {
		t.Error("empty timings must render empty")
	}
}

// A cache entry built for an unrestricted viewer must never answer a
// restricted one, whichever of them asks first.
func TestDashboardCacheNeverLeaksAcrossScopes(t *testing.T) {
	rt, tok, cookie := newScopedListFixture(t)
	rt.dash = newDashboardCache(time.Minute, time.Minute)

	for _, path := range []string{"/api/v1/apps/resource-usage", "/api/v1/apps-metrics"} {
		full := httptest.NewRecorder()
		rt.Handler().ServeHTTP(full, authedRequest(t, cookie, http.MethodGet, path, ""))
		if !strings.Contains(full.Body.String(), "beta-app") {
			t.Fatalf("%s: admin body missing beta-app: %s", path, full.Body.String())
		}

		scoped := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		bearer(tok)(req)
		rt.Handler().ServeHTTP(scoped, req)
		if scoped.Code != http.StatusOK || strings.Contains(scoped.Body.String(), "beta-app") {
			t.Errorf("%s: scoped viewer saw cached admin data: %d %s", path, scoped.Code, scoped.Body.String())
		}
		if !strings.Contains(scoped.Body.String(), "alpha-app") {
			t.Errorf("%s: scoped body missing alpha-app: %s", path, scoped.Body.String())
		}
	}
}
