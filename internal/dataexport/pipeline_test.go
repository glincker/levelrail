package dataexport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testDeliverer(cfg Config) (*deliverer, *[]time.Duration) {
	d := newDeliverer(cfg)
	now := time.Unix(1_700_000_000, 0)
	var sleeps []time.Duration
	d.now = func() time.Time { return now }
	d.jitter = func() float64 { return 1 }
	d.sleep = func(_ context.Context, dur time.Duration) error {
		sleeps = append(sleeps, dur)
		now = now.Add(dur)
		return nil
	}
	return d, &sleeps
}

func TestQueueDropsOldestOnOverflow(t *testing.T) {
	q := newQueue[int](3)
	dropped := 0
	for i := 1; i <= 5; i++ {
		dropped += q.push(i)
	}
	if dropped != 2 || q.len() != 3 {
		t.Fatalf("dropped=%d len=%d, want 2 and 3", dropped, q.len())
	}
	got := q.take(10, 1<<20, func(int) int { return 1 })
	if len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Fatalf("take = %v, want [3 4 5]", got)
	}
}

func TestQueueTakeRespectsByteBudget(t *testing.T) {
	q := newQueue[int](10)
	for _, v := range []int{40, 40, 40} {
		q.push(v)
	}
	got := q.take(10, 100, func(v int) int { return v })
	if len(got) != 2 {
		t.Fatalf("take = %v, want two items under a 100 byte budget", got)
	}
	big := newQueue[int](10)
	big.push(500)
	if got := big.take(10, 100, func(v int) int { return v }); len(got) != 1 {
		t.Fatalf("an oversized item must still go out alone, got %v", got)
	}
}

func TestDeliverRetriesWithBackoff(t *testing.T) {
	tests := []struct {
		name       string
		failures   int
		attempts   int
		wantErr    bool
		wantSleeps []time.Duration
		wantCalls  int
	}{
		{"first try", 0, 4, false, nil, 1},
		{"recovers on third", 2, 4, false, []time.Duration{time.Second, 2 * time.Second}, 3},
		{"exhausted", 10, 3, true, []time.Duration{time.Second, 2 * time.Second}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxAttempts, cfg.BackoffBase, cfg.BackoffMax, cfg.BreakerFailures = tt.attempts, time.Second, 30*time.Second, 100
			d, sleeps := testDeliverer(cfg)
			calls := 0
			err := d.do(context.Background(), 5, func(context.Context) error {
				calls++
				if calls <= tt.failures {
					return errors.New("boom")
				}
				return nil
			})
			if (err != nil) != tt.wantErr || calls != tt.wantCalls {
				t.Fatalf("err=%v calls=%d, want err=%v calls=%d", err, calls, tt.wantErr, tt.wantCalls)
			}
			if len(*sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleeps = %v, want %v", *sleeps, tt.wantSleeps)
			}
			for i := range tt.wantSleeps {
				if (*sleeps)[i] != tt.wantSleeps[i] {
					t.Fatalf("sleeps = %v, want %v", *sleeps, tt.wantSleeps)
				}
			}
			snap := d.st.snapshot()
			if !tt.wantErr && snap.Sent != 5 {
				t.Fatalf("sent = %d, want 5", snap.Sent)
			}
			if snap.Failures != int64(min(tt.failures, tt.attempts)) {
				t.Fatalf("failures = %d", snap.Failures)
			}
		})
	}
}

func TestBackoffIsCapped(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BackoffBase, cfg.BackoffMax = time.Second, 5*time.Second
	d, _ := testDeliverer(cfg)
	if got := d.backoff(10); got != 5*time.Second {
		t.Fatalf("backoff(10) = %v, want 5s", got)
	}
}

func TestDeliverStopsOnPermanentError(t *testing.T) {
	d, sleeps := testDeliverer(DefaultConfig())
	calls := 0
	err := d.do(context.Background(), 1, func(context.Context) error {
		calls++
		return permanent(errors.New("400"))
	})
	if err == nil || calls != 1 || len(*sleeps) != 0 {
		t.Fatalf("err=%v calls=%d sleeps=%v, want one call and no retry", err, calls, *sleeps)
	}
}

func TestBreakerOpensThenAllowsSingleTrial(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts, cfg.BreakerFailures, cfg.BreakerCooldown, cfg.BackoffBase = 2, 3, time.Minute, time.Millisecond
	d, sleeps := testDeliverer(cfg)
	fail := func(context.Context) error { return errors.New("down") }

	_ = d.do(context.Background(), 1, fail)
	_ = d.do(context.Background(), 1, fail)
	if !d.st.snapshot().CircuitOpen {
		t.Fatal("breaker should be open after 3 consecutive failures")
	}

	*sleeps = nil
	calls := 0
	_ = d.do(context.Background(), 1, func(context.Context) error { calls++; return errors.New("still down") })
	if calls != 1 {
		t.Fatalf("half-open must allow exactly one trial, got %d calls", calls)
	}
	if len(*sleeps) == 0 || (*sleeps)[0] > time.Minute {
		t.Fatalf("expected to wait out the cooldown first, sleeps=%v", *sleeps)
	}

	err := d.do(context.Background(), 1, func(context.Context) error { return nil })
	if err != nil || d.st.snapshot().CircuitOpen {
		t.Fatalf("a successful trial must close the breaker, err=%v", err)
	}
}

func TestBatcherDropsBatchAfterExhaustionAndCounts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts, cfg.BatchLines, cfg.FlushInterval, cfg.BreakerFailures = 1, 2, 0, 100
	d, _ := testDeliverer(cfg)
	q := newQueue[int](100)
	d.st.depth = q.len
	var sent [][]int
	fail := true
	b := &batcher[int]{q: q, d: d, size: func(int) int { return 1 }, send: func(_ context.Context, items []int) error {
		if fail {
			fail = false
			return errors.New("first batch fails")
		}
		sent = append(sent, items)
		return nil
	}}
	for i := 1; i <= 4; i++ {
		q.push(i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for d.st.snapshot().Sent != 2 {
		select {
		case <-deadline:
			t.Fatalf("batches not delivered: %+v", d.st.snapshot())
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	snap := d.st.snapshot()
	if snap.Dropped != 2 || len(sent) != 1 || sent[0][0] != 3 {
		t.Fatalf("dropped=%d sent=%v, want first batch dropped and second delivered", snap.Dropped, sent)
	}
}
