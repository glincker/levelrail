package testenv

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Test-only retry tuning for registry pulls. Production pulls are not retried here.
const (
	pullMaxAttempts = 4
	pullBaseDelay   = 2 * time.Second
	pullMaxDelay    = 20 * time.Second
	pullTotalCap    = 90 * time.Second
)

// retryingRuntime retries Create, which is where the real runtime pulls
// the image, so pull-time 429s and transient network errors are retried
// for every live test that builds its env through NewLiveBuildEnv.
type retryingRuntime struct {
	docker.Runtime
	t *testing.T
}

// Create retries the underlying Create on retryable pull errors.
func (r retryingRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	return createWithRetry(ctx, r.t, r.Runtime.Create, spec)
}

// retryingClient embeds the concrete client so every optional interface the
// reconciler type-asserts (exit state, networks, logs) stays visible.
type retryingClient struct {
	*docker.Client
	t *testing.T
}

// Create retries the client's Create on retryable pull errors.
func (r retryingClient) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	return createWithRetry(ctx, r.t, r.Client.Create, spec)
}

// ReportsExitTimes is true because the local client fills start and finish times.
func (r retryingClient) ReportsExitTimes() bool { return true }

func createWithRetry(ctx context.Context, t *testing.T, create func(context.Context, docker.ContainerSpec) (string, error), spec docker.ContainerSpec) (string, error) {
	var id string
	err := retryPull(ctx, sleepCtx, func() error {
		var createErr error
		id, createErr = create(ctx, spec)
		if createErr != nil && isRetryablePullError(createErr) {
			t.Logf("registry pull for %q failed, retrying: %v", spec.Image, createErr)
		}
		return createErr
	})
	return id, err
}

func withPullRetry(t *testing.T, rt docker.Runtime) docker.Runtime {
	if c, ok := rt.(*docker.Client); ok {
		return retryingClient{Client: c, t: t}
	}
	return retryingRuntime{Runtime: rt, t: t}
}

// isRetryablePullError reports whether err is a rate limit or a transient
// network failure. Not-found errors are never retried.
func isRetryablePullError(err error) bool {
	if err == nil || cerrdefs.IsNotFound(err) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"429", "too many requests", "toomanyrequests", "rate limit",
		"connection reset", "connection refused", "i/o timeout", "tls handshake timeout",
		"unexpected eof", "timeout", "temporary failure",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// retryPull runs op up to pullMaxAttempts times, sleeping between attempts
// with exponential backoff and full jitter. It stops early once pullTotalCap
// has elapsed, the error is not retryable, or ctx is done.
func retryPull(ctx context.Context, sleep func(context.Context, time.Duration) error, op func() error) error {
	start := time.Now()
	var err error
	for attempt := 1; attempt <= pullMaxAttempts; attempt++ {
		if err = op(); err == nil || !isRetryablePullError(err) {
			return err
		}
		if attempt == pullMaxAttempts {
			break
		}
		delay := pullBackoff(attempt)
		if time.Since(start)+delay > pullTotalCap {
			break
		}
		if sleepErr := sleep(ctx, delay); sleepErr != nil {
			return fmt.Errorf("registry pull retry stopped: %w (last error: %w)", sleepErr, err)
		}
	}
	return fmt.Errorf("registry pull failed after retries: %w", err)
}

// pullBackoff returns the jittered delay before retry attempt+1: half to
// full of min(pullBaseDelay*2^(attempt-1), pullMaxDelay).
func pullBackoff(attempt int) time.Duration {
	d := pullBaseDelay << (attempt - 1)
	if d > pullMaxDelay {
		d = pullMaxDelay
	}
	half := d / 2
	//nolint:gosec,forbidigo // jitter only, not security sensitive
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
