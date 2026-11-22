package cloudspend

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testCache(cfg Config) (*Cache, *clock) {
	clk := &clock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	cfg.Now = clk.now
	return NewCache(func() Config { return cfg }), clk
}

func okFetch(n *atomic.Int32) Fetch {
	return func(context.Context) (SpendReport, error) {
		n.Add(1)
		return newReport("acc", ProviderOracle, time.Time{}, time.Time{}, SourceUsageAPI), nil
	}
}

func TestCacheRefreshInsideIntervalIsRejected(t *testing.T) {
	c, clk := testCache(Config{CacheTTL: time.Hour, RefreshMinInterval: 10 * time.Minute, RefreshPerUserPerHour: 2})
	var n atomic.Int32
	k := Key{AccountID: "acc", Scope: "mtd"}
	if _, d, err := c.Refresh(context.Background(), k, "u1", okFetch(&n)); err != nil || !d.Allowed {
		t.Fatalf("first refresh: %+v %v", d, err)
	}
	clk.add(3 * time.Minute)
	// A different scope must not dodge the per-account interval.
	rep, d, _ := c.Refresh(context.Background(), Key{AccountID: "acc", Scope: "other"}, "u1", okFetch(&n))
	if d.Allowed || d.Reason != ReasonAccountInterval || d.RetryAfter != 7*time.Minute {
		t.Fatalf("want account_interval 7m, got %+v", d)
	}
	_ = rep
	if n.Load() != 1 {
		t.Fatalf("fetches = %d", n.Load())
	}
	clk.add(8 * time.Minute)
	if _, d, _ := c.Refresh(context.Background(), k, "u1", okFetch(&n)); !d.Allowed {
		t.Fatalf("after interval: %+v", d)
	}
	clk.add(11 * time.Minute)
	if _, d, _ := c.Refresh(context.Background(), k, "u1", okFetch(&n)); d.Allowed || d.Reason != ReasonUserHourly {
		t.Fatalf("want user_hourly, got %+v", d)
	}
}

func TestCacheSingleFlight(t *testing.T) {
	c, _ := testCache(Config{CacheTTL: time.Hour})
	var n atomic.Int32
	release := make(chan struct{})
	inner := okFetch(&n)
	fetch := func(ctx context.Context) (SpendReport, error) {
		<-release
		return inner(ctx)
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), Key{AccountID: "acc"}, fetch); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("fetches = %d, want 1", n.Load())
	}
}

func TestCacheKeepsLastGoodOnError(t *testing.T) {
	c, clk := testCache(Config{CacheTTL: time.Minute})
	k := Key{AccountID: "acc"}
	var n atomic.Int32
	if _, err := c.Get(context.Background(), k, okFetch(&n)); err != nil {
		t.Fatal(err)
	}
	clk.add(2 * time.Minute)
	failing := func(context.Context) (SpendReport, error) { return SpendReport{}, &ProviderError{Status: 403} }
	stale, _ := c.Get(context.Background(), k, failing)
	if !stale.Stale {
		t.Fatal("expired entry must be marked stale")
	}
	time.Sleep(50 * time.Millisecond)
	got, err := c.Get(context.Background(), k, failing)
	if err != nil || got.ErrorCode != "auth" || !got.Stale || got.AccountID != "acc" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestTotalsKeepCurrenciesApart(t *testing.T) {
	got := TotalsByCurrency([]Line{{Amount: 1, Currency: "USD"}, {Amount: 2, Currency: "EUR"}, {Amount: 3, Currency: "USD"}})
	if len(got) != 2 || got[0].Currency != "EUR" || got[1].Amount != 4 {
		t.Fatalf("got %+v", got)
	}
}
