package scheduledtask

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func noSleepJob(delays *[]time.Duration, mu *sync.Mutex) *OneShot {
	return &OneShot{Sleep: func(_ context.Context, d time.Duration) error {
		mu.Lock()
		*delays = append(*delays, d)
		mu.Unlock()
		return nil
	}}
}

func TestOneShot_CompletionsAllSucceed(t *testing.T) {
	var calls atomic.Int32
	job := &OneShot{}
	snap := job.Run(context.Background(), OneShotSpec{Completions: 5, Parallelism: 2}, func(_ context.Context, _, _ int) AttemptOutcome {
		calls.Add(1)
		return AttemptOutcome{RunID: "r", Status: store.ScheduledTaskStatusSuccess}
	})
	if snap.State != OneShotSucceeded || snap.Succeeded != 5 || calls.Load() != 5 || len(snap.RunIDs) != 5 {
		t.Fatalf("snap %+v calls %d", snap, calls.Load())
	}
}

func TestOneShot_ParallelismCap(t *testing.T) {
	var cur, peak atomic.Int32
	job := &OneShot{}
	job.Run(context.Background(), OneShotSpec{Completions: 8, Parallelism: 3}, func(_ context.Context, _, _ int) AttemptOutcome {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		return AttemptOutcome{Status: store.ScheduledTaskStatusSuccess}
	})
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("peak parallelism %d, want 2..3", peak.Load())
	}
}

func TestOneShot_RetriesWithExponentialBackoff(t *testing.T) {
	var delays []time.Duration
	var mu sync.Mutex
	job := noSleepJob(&delays, &mu)
	snap := job.Run(context.Background(), OneShotSpec{Completions: 1, Parallelism: 1, BackoffLimit: 3, BackoffBase: time.Second, BackoffMax: time.Minute},
		func(_ context.Context, _, attempt int) AttemptOutcome {
			if attempt < 3 {
				return AttemptOutcome{Status: store.ScheduledTaskStatusFailed}
			}
			return AttemptOutcome{Status: store.ScheduledTaskStatusSuccess}
		})
	if snap.State != OneShotSucceeded || snap.FailedAttempts != 2 {
		t.Fatalf("snap %+v", snap)
	}
	if len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
		t.Fatalf("delays %v", delays)
	}
}

func TestOneShot_BackoffLimitExhaustedFails(t *testing.T) {
	var delays []time.Duration
	var mu sync.Mutex
	job := noSleepJob(&delays, &mu)
	var calls atomic.Int32
	snap := job.Run(context.Background(), OneShotSpec{Completions: 3, Parallelism: 1, BackoffLimit: 2, BackoffBase: time.Second, BackoffMax: time.Minute},
		func(_ context.Context, _, _ int) AttemptOutcome {
			calls.Add(1)
			return AttemptOutcome{Status: store.ScheduledTaskStatusFailed}
		})
	if snap.State != OneShotFailed || snap.Succeeded != 0 || snap.FailedAttempts != 3 || calls.Load() != 3 {
		t.Fatalf("snap %+v calls %d", snap, calls.Load())
	}
	if snap.FinishedAt == nil || snap.Message == "" {
		t.Fatalf("missing finish info: %+v", snap)
	}
}

func TestOneShot_SkippedConcurrencyDoesNotSpendBudget(t *testing.T) {
	var delays []time.Duration
	var mu sync.Mutex
	job := noSleepJob(&delays, &mu)
	snap := job.Run(context.Background(), OneShotSpec{Completions: 1, Parallelism: 1, BackoffLimit: 0, BackoffBase: time.Second},
		func(_ context.Context, _, attempt int) AttemptOutcome {
			if attempt == 1 {
				return AttemptOutcome{Status: store.ScheduledTaskStatusSkippedConcurrency}
			}
			return AttemptOutcome{Status: store.ScheduledTaskStatusSuccess}
		})
	if snap.State != OneShotSucceeded || snap.FailedAttempts != 0 {
		t.Fatalf("snap %+v", snap)
	}
}

func TestOneShotSpec_Validate(t *testing.T) {
	l := OneShotLimits{MaxCompletions: 10, MaxParallelism: 4}
	s := OneShotSpec{}
	if err := s.Validate(l); err != nil || s.Completions != 1 || s.Parallelism != 1 {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	s = OneShotSpec{Completions: 20}
	if s.Validate(l) == nil {
		t.Fatal("completions over the limit accepted")
	}
	s = OneShotSpec{Completions: 10, Parallelism: 9}
	if s.Validate(l) == nil {
		t.Fatal("parallelism over the limit accepted")
	}
}

func TestOneShotLimitsFromEnv(t *testing.T) {
	env := map[string]string{"APP_JOB_DEFAULT_BACKOFF_LIMIT": "0", "APP_JOB_DEFAULT_TIMEOUT": "90s"}
	l := OneShotLimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if l.BackoffLimit != 0 || l.Timeout != 90*time.Second || l.MaxCompletions != 100 {
		t.Fatalf("%+v", l)
	}
}
