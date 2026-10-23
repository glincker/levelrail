package apiclient

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

const headerIdempotencyKey = "Idempotency-Key"

type idempotencyKeyCtx struct{}

// WithIdempotencyKey makes every request sent with the returned context carry
// an Idempotency-Key header, which also makes it safe to retry.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, idempotencyKeyCtx{}, key)
}

func idempotencyKeyFrom(ctx context.Context) string {
	k, _ := ctx.Value(idempotencyKeyCtx{}).(string)
	return k
}

// RetryPolicy bounds the retry of HTTP 429 and 503. Zero MaxRetries disables it.
type RetryPolicy struct {
	MaxRetries int           // retries after the first attempt
	MaxWait    time.Duration // total time spent waiting across all retries
	BaseDelay  time.Duration // backoff start when the server sends no Retry-After
}

// WithRetry enables bounded retry for idempotent requests and requests carrying an Idempotency-Key.
func WithRetry(p RetryPolicy) Option {
	return func(c *Client) { c.retry = p }
}

// WithSleep replaces the wait between retries, so tests never really sleep.
func WithSleep(sleep func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.sleep = sleep }
}

// retrySafe: a plain POST or PATCH is never retried unless it carries an Idempotency-Key.
func retrySafe(method, idempotencyKey string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return idempotencyKey != ""
}

// retryDelay honors Retry-After (never shortened by jitter, which only adds up to 25%), else doubles BaseDelay per attempt.
func (p RetryPolicy) retryDelay(retryAfter time.Duration, attempt int) time.Duration {
	d := retryAfter
	if d <= 0 {
		d = p.BaseDelay << attempt
	}
	return d + time.Duration(rand.Int64N(int64(d)/4+1)) //nolint:gosec,forbidigo // jitter only, not security
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
