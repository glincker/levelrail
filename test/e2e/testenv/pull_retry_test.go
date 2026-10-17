package testenv

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
)

func TestIsRetryablePullError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "rate limit 429 from daemon", err: errors.New("docker: pull image \"x\": Error response from daemon: toomanyrequests: Rate exceeded"), want: true},
		{name: "rate limit 429 status", err: errors.New("received unexpected HTTP status: 429 Too Many Requests"), want: true},
		{name: "connection reset", err: errors.New("read tcp: connection reset by peer"), want: true},
		{name: "not found sentinel", err: fmt.Errorf("pull: %w", cerrdefs.ErrNotFound), want: false},
		{name: "not found message", err: errors.New("manifest unknown: not found"), want: false},
		{name: "context canceled", err: fmt.Errorf("pull: %w", context.Canceled), want: false},
		{name: "context deadline", err: fmt.Errorf("pull: %w", context.DeadlineExceeded), want: false},
		{name: "unrelated error", err: errors.New("invalid reference format"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isRetryablePullError(tt.err); got != tt.want {
				t.Fatalf("isRetryablePullError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryPull(t *testing.T) {
	t.Parallel()

	rateLimited := errors.New("toomanyrequests: Rate exceeded")
	notFound := fmt.Errorf("pull: %w", cerrdefs.ErrNotFound)
	noSleep := func(context.Context, time.Duration) error { return nil }

	t.Run("retries 429 until success", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retryPull(context.Background(), noSleep, func() error {
			calls++
			if calls < 3 {
				return rateLimited
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatalf("retryPull() = %v after %d calls, want nil after 3", err, calls)
		}
	})

	t.Run("gives up after max attempts", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retryPull(context.Background(), noSleep, func() error {
			calls++
			return rateLimited
		})
		if !errors.Is(err, rateLimited) || calls != pullMaxAttempts {
			t.Fatalf("retryPull() = %v after %d calls, want rate limit error after %d", err, calls, pullMaxAttempts)
		}
	})

	t.Run("does not retry not-found", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retryPull(context.Background(), noSleep, func() error {
			calls++
			return notFound
		})
		if !errors.Is(err, notFound) || calls != 1 {
			t.Fatalf("retryPull() = %v after %d calls, want not-found after 1", err, calls)
		}
	})

	t.Run("context cancel stops retries", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := retryPull(ctx, sleepCtx, func() error {
			calls++
			return rateLimited
		})
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("retryPull() = %v after %d calls, want context.Canceled after 1", err, calls)
		}
	})
}

func TestPullBackoffBounds(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt < pullMaxAttempts; attempt++ {
		ceiling := pullBaseDelay << (attempt - 1)
		if ceiling > pullMaxDelay {
			ceiling = pullMaxDelay
		}
		for range 50 {
			d := pullBackoff(attempt)
			if d < ceiling/2 || d > ceiling {
				t.Fatalf("pullBackoff(%d) = %v, want within [%v, %v]", attempt, d, ceiling/2, ceiling)
			}
		}
	}
}
