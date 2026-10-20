package gitprovider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   string
		want time.Duration
		ok   bool
	}{
		{"seconds", "7", 7 * time.Second, true},
		{"empty", "", 0, false},
		{"garbage", "soon", 0, false},
		{"http date", now.Add(90 * time.Second).Format(time.RFC1123), 90 * time.Second, true},
		{"past date", now.Add(-time.Minute).Format(time.RFC1123), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseRetryAfter(tt.in, now)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseRetryAfter(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func limited(retryAfter string) error {
	return &APIError{Prefix: "x", API: "x", StatusCode: 429, RetryAfter: retryAfter, RateLimited: true}
}

func TestWithRetry(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		errs      []error
		policy    RetryPolicy
		wantCalls int
		wantSleep []time.Duration
		wantRL    bool
		wantNext  time.Time
	}{
		{name: "succeeds first time", errs: []error{nil}, policy: RetryPolicy{Retries: 2, MaxWait: 5 * time.Second}, wantCalls: 1},
		{name: "retries within max wait", errs: []error{limited("2"), nil}, policy: RetryPolicy{Retries: 2, MaxWait: 5 * time.Second}, wantCalls: 2, wantSleep: []time.Duration{2 * time.Second}},
		{name: "retry budget exhausted", errs: []error{limited("1"), limited("1"), limited("1")}, policy: RetryPolicy{Retries: 1, MaxWait: 5 * time.Second}, wantCalls: 2, wantSleep: []time.Duration{time.Second}, wantRL: true, wantNext: now.Add(time.Second)},
		{name: "retry after beyond max wait is not slept on", errs: []error{limited("120")}, policy: RetryPolicy{Retries: 3, MaxWait: 5 * time.Second}, wantCalls: 1, wantRL: true, wantNext: now.Add(2 * time.Minute)},
		{name: "missing header uses default backoff", errs: []error{limited("")}, policy: RetryPolicy{Retries: 3, MaxWait: 5 * time.Second, DefaultBackoff: time.Minute}, wantCalls: 1, wantRL: true, wantNext: now.Add(time.Minute)},
		{name: "other errors pass through", errs: []error{errors.New("boom")}, policy: RetryPolicy{Retries: 3, MaxWait: 5 * time.Second}, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var slept []time.Duration
			p := tt.policy
			p.Now = func() time.Time { return now }
			p.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
			calls := 0
			err := WithRetry(context.Background(), p, "github", func() error {
				e := tt.errs[min(calls, len(tt.errs)-1)]
				calls++
				return e
			})
			if calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if len(slept) != len(tt.wantSleep) {
				t.Fatalf("slept %v, want %v", slept, tt.wantSleep)
			}
			rl, isRL := AsRateLimit(err)
			if isRL != tt.wantRL {
				t.Fatalf("rate limit error = %v (%v), want %v", isRL, err, tt.wantRL)
			}
			if isRL && !rl.NextCheck.Equal(tt.wantNext) {
				t.Fatalf("NextCheck = %v, want %v", rl.NextCheck, tt.wantNext)
			}
		})
	}
}

func TestWithRetryStopsOnCancelledSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := RetryPolicy{Retries: 2, MaxWait: time.Minute}
	err := WithRetry(ctx, p, "github", func() error { return limited("1") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
