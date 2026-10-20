package ai

import (
	"context"
	"math/rand/v2"
	"time"
)

// RetryPolicy controls retry of retryable provider failures. Sleep and
// Jitter are injectable so tests run on a fake clock.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Jitter returns a value in [0,1); nil uses math/rand.
	Jitter func() float64
	// Sleep waits d or until ctx ends; nil uses a real timer.
	Sleep func(ctx context.Context, d time.Duration) error
}

func (p RetryPolicy) sleep(ctx context.Context, d time.Duration) error {
	if p.Sleep != nil {
		return p.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// delay is Retry-After when given (and within MaxDelay), else exponential
// backoff with jitter in [d/2, d). ok is false when the server asked for a
// wait longer than we will hold the turn open.
func (p RetryPolicy) delay(attempt int, retryAfter time.Duration) (time.Duration, bool) {
	if retryAfter > 0 {
		if p.MaxDelay > 0 && retryAfter > p.MaxDelay {
			return 0, false
		}
		return retryAfter, true
	}
	d := p.BaseDelay << attempt
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	j := rand.Float64() //nolint:gosec,forbidigo // jitter only, not security sensitive
	if p.Jitter != nil {
		j = p.Jitter()
	}
	return d/2 + time.Duration(j*float64(d/2)), true
}

// RetryProvider wraps a Provider, retrying retryable failures that happened
// before any output reached the caller (retrying later would duplicate text).
type RetryProvider struct {
	Inner  Provider
	Policy RetryPolicy
}

// Complete implements Provider.
func (r *RetryProvider) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	streamed := false
	wrapped := func(s string) {
		streamed = true
		if onText != nil {
			onText(s)
		}
	}
	attempts := max(r.Policy.MaxAttempts, 1)
	for attempt := 0; ; attempt++ {
		res, err := r.Inner.Complete(ctx, req, wrapped)
		if err == nil {
			return res, nil
		}
		pe, ok := AsProviderError(err)
		if !ok || !pe.Retryable() || streamed || attempt+1 >= attempts || ctx.Err() != nil {
			return res, err
		}
		d, ok := r.Policy.delay(attempt, pe.RetryAfter)
		if !ok {
			return res, err
		}
		if serr := r.Policy.sleep(ctx, d); serr != nil {
			return res, err
		}
	}
}
