package secretprovider

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRateLimited means the per provider request budget would not free up within the read timeout.
var ErrRateLimited = errors.New("provider request rate limit reached")

type bucket struct {
	tokens float64
	last   time.Time
}

// limiter is a token bucket per provider name, so one noisy provider cannot starve another.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

func newLimiter() *limiter { return &limiter{buckets: map[string]*bucket{}} }

// reserve takes one token and returns how long the caller must wait before using it.
// rps <= 0 disables limiting. Burst equals one second of budget.
func (l *limiter) reserve(name string, rps float64, now time.Time) time.Duration {
	if rps <= 0 {
		return 0
	}
	burst := rps
	if burst < 1 {
		burst = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[name]
	if !ok {
		b = &bucket{tokens: burst, last: now}
		l.buckets[name] = b
	}
	if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens += el * rps
		if b.tokens > burst {
			b.tokens = burst
		}
		b.last = now
	}
	b.tokens--
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(-b.tokens / rps * float64(time.Second))
}

// wait blocks for the reserved delay, or fails closed when it exceeds the read timeout.
func (r *Resolver) wait(ctx context.Context, name string, t Tunables) error {
	d := r.limit.reserve(name, t.MaxRPS, r.now())
	if d <= 0 {
		return nil
	}
	if d > t.Timeout {
		return ErrRateLimited
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
