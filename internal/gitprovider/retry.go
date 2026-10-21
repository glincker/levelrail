package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	envRateLimitRetries = "APP_GIT_RATE_LIMIT_RETRIES"
	envRateLimitMaxWait = "APP_GIT_RATE_LIMIT_MAX_WAIT"
	envRateLimitBackoff = "APP_GIT_RATE_LIMIT_BACKOFF"

	defaultRateLimitRetries = 2
	defaultRateLimitMaxWait = 5 * time.Second
	defaultRateLimitBackoff = 60 * time.Second
)

// RetryPolicy bounds how a rate-limited provider call is retried inside one
// request. A Retry-After longer than MaxWait is not slept on: the caller
// gets a RateLimitError carrying the next-check time instead.
type RetryPolicy struct {
	Retries        int
	MaxWait        time.Duration
	DefaultBackoff time.Duration
	Sleep          func(ctx context.Context, d time.Duration) error
	Now            func() time.Time
}

// RetryPolicyFromEnv reads the retry bounds from APP_GIT_RATE_LIMIT_*.
func RetryPolicyFromEnv() RetryPolicy {
	p := RetryPolicy{Retries: defaultRateLimitRetries, MaxWait: defaultRateLimitMaxWait, DefaultBackoff: defaultRateLimitBackoff}
	if n, err := strconv.Atoi(os.Getenv(envRateLimitRetries)); err == nil && n >= 0 {
		p.Retries = n
	}
	if d, err := time.ParseDuration(os.Getenv(envRateLimitMaxWait)); err == nil && d >= 0 {
		p.MaxWait = d
	}
	if d, err := time.ParseDuration(os.Getenv(envRateLimitBackoff)); err == nil && d > 0 {
		p.DefaultBackoff = d
	}
	return p
}

// RateLimitError is returned once a bounded retry gave up on a rate limit.
// NextCheck is when the provider said (or the default backoff implies) a
// call may succeed again.
type RateLimitError struct {
	Provider   string
	RetryAfter time.Duration
	NextCheck  time.Time
	Err        error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s rate limit: next check at %s: %v", e.Provider, e.NextCheck.UTC().Format(time.RFC3339), e.Err)
}

func (e *RateLimitError) Unwrap() error { return e.Err }

// AsRateLimit reports whether err is (or wraps) a RateLimitError.
func AsRateLimit(err error) (*RateLimitError, bool) {
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return rl, true
	}
	return nil, false
}

// ParseRetryAfter reads a Retry-After header: delta seconds or an HTTP date.
func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for rate limit: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// WithRetry runs fn, retrying a rate-limited failure up to p.Retries times
// when the provider's Retry-After fits within p.MaxWait. Otherwise it
// returns a *RateLimitError with the next-check time. Other errors pass
// through unchanged.
func WithRetry(ctx context.Context, p RetryPolicy, provider string, fn func() error) error {
	now := p.Now
	if now == nil {
		now = time.Now
	}
	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.RateLimited {
			return err
		}
		wait, ok := ParseRetryAfter(apiErr.RetryAfter, now())
		if !ok {
			wait = p.DefaultBackoff
		}
		if attempt >= p.Retries || wait > p.MaxWait {
			return &RateLimitError{Provider: provider, RetryAfter: wait, NextCheck: now().Add(wait), Err: err}
		}
		if err := sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// ListOptions bounds a paginated repository listing.
type ListOptions struct {
	// MaxItems stops paging once this many items are collected (0 = no cap).
	MaxItems int
	Retry    RetryPolicy
}

// Reached reports whether n collected items meets the cap.
func (o ListOptions) Reached(n int) bool { return o.MaxItems > 0 && n >= o.MaxItems }

// ListResult reports how a paginated listing ended.
type ListResult struct {
	// Truncated is true when the MaxItems cap stopped paging while the
	// provider still had more.
	Truncated bool
}
