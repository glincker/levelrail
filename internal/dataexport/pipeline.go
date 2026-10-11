package dataexport

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// queue is a bounded FIFO that drops its oldest items on overflow, so a slow
// or dead destination can only ever cost a fixed amount of memory.
type queue[T any] struct {
	mu     sync.Mutex
	items  []T
	head   int
	cap    int
	notify chan struct{}
}

func newQueue[T any](capacity int) *queue[T] {
	return &queue[T]{cap: capacity, notify: make(chan struct{}, 1)}
}

// push appends v and reports how many older items were dropped to fit it.
func (q *queue[T]) push(v T) int {
	q.mu.Lock()
	q.items = append(q.items, v)
	dropped := 0
	for len(q.items)-q.head > q.cap {
		var zero T
		q.items[q.head] = zero
		q.head++
		dropped++
	}
	if q.head > 1024 && q.head*2 > len(q.items) {
		q.items = append([]T(nil), q.items[q.head:]...)
		q.head = 0
	}
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return dropped
}

func (q *queue[T]) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) - q.head
}

// take removes up to maxItems items whose sizes sum to at most maxBytes (an
// item larger than maxBytes still goes out alone).
func (q *queue[T]) take(maxItems, maxBytes int, size func(T) int) []T {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []T
	total := 0
	for q.head < len(q.items) && len(out) < maxItems {
		s := size(q.items[q.head])
		if len(out) > 0 && total+s > maxBytes {
			break
		}
		out = append(out, q.items[q.head])
		var zero T
		q.items[q.head] = zero
		q.head++
		total += s
	}
	return out
}

// breaker opens after consecutive failed batches and stays open for a
// cooldown, after which exactly one trial attempt is allowed.
type breaker struct {
	mu          sync.Mutex
	threshold   int
	cooldown    time.Duration
	consecutive int
	openUntil   time.Time
	open        atomic.Bool
}

// block returns how long the caller must wait before attempting, and whether
// the next attempt is a half-open trial.
func (b *breaker) block(now time.Time) (wait time.Duration, trial bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.consecutive < b.threshold {
		return 0, false
	}
	if now.Before(b.openUntil) {
		return b.openUntil.Sub(now), true
	}
	return 0, true
}

func (b *breaker) success() {
	b.mu.Lock()
	b.consecutive = 0
	b.mu.Unlock()
	b.open.Store(false)
}

func (b *breaker) failure(now time.Time) {
	b.mu.Lock()
	b.consecutive++
	if b.consecutive >= b.threshold {
		b.openUntil = now.Add(b.cooldown)
		b.open.Store(true)
	}
	b.mu.Unlock()
}

// Snapshot is a point-in-time copy of a destination's delivery counters.
type Snapshot struct {
	Sent        int64
	Dropped     int64
	Failures    int64
	QueueDepth  int64
	CircuitOpen bool
	LastSuccess time.Time
	LastError   string
	LastErrorAt time.Time
	CursorAt    string
	CursorID    string
}

type stats struct {
	sent, dropped, failures atomic.Int64
	depth                   func() int
	br                      *breaker
	mu                      sync.Mutex
	lastSuccess, lastErrAt  time.Time
	lastErr                 string
	cursorAt, cursorID      string
}

func (s *stats) snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{
		Sent: s.sent.Load(), Dropped: s.dropped.Load(), Failures: s.failures.Load(),
		LastSuccess: s.lastSuccess, LastError: s.lastErr, LastErrorAt: s.lastErrAt,
		CursorAt: s.cursorAt, CursorID: s.cursorID,
	}
	if s.depth != nil {
		out.QueueDepth = int64(s.depth())
	}
	if s.br != nil {
		out.CircuitOpen = s.br.open.Load()
	}
	return out
}

func (s *stats) recordSuccess(n int, at time.Time) {
	s.sent.Add(int64(n))
	s.mu.Lock()
	s.lastSuccess = at
	s.lastErr = ""
	s.mu.Unlock()
}

func (s *stats) recordFailure(err error, at time.Time) {
	s.failures.Add(1)
	s.mu.Lock()
	s.lastErr = truncate(untrusted.RedactTokens(err.Error()), 300)
	s.lastErrAt = at
	s.mu.Unlock()
}

func (s *stats) setCursor(at, id string) {
	s.mu.Lock()
	s.cursorAt, s.cursorID = at, id
	s.mu.Unlock()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// permanentError marks a failure that retrying cannot fix, such as an HTTP 4xx.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

func permanent(err error) error { return &permanentError{err: err} }

// deliverer runs one send function with retry, backoff, jitter and the
// circuit breaker. now, sleep and jitter are injectable for tests.
type deliverer struct {
	cfg    Config
	br     *breaker
	st     *stats
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	jitter func() float64
}

func newDeliverer(cfg Config) *deliverer {
	br := &breaker{threshold: cfg.BreakerFailures, cooldown: cfg.BreakerCooldown}
	return &deliverer{
		cfg: cfg, br: br, st: &stats{br: br},
		now:    time.Now,
		sleep:  sleepCtx,
		jitter: func() float64 { return 0.8 + 0.4*rand.Float64() }, //nolint:gosec // retry jitter, not a security value
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
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

func (d *deliverer) backoff(attempt int) time.Duration {
	delay := d.cfg.BackoffBase
	for i := 1; i < attempt && delay < d.cfg.BackoffMax; i++ {
		delay *= 2
	}
	if delay > d.cfg.BackoffMax {
		delay = d.cfg.BackoffMax
	}
	return time.Duration(float64(delay) * d.jitter())
}

// do sends count items through fn. It returns nil on delivery and the last
// error when the attempts are exhausted, the failure is permanent, or ctx ends.
func (d *deliverer) do(ctx context.Context, count int, fn func(context.Context) error) error {
	wait, trial := d.br.block(d.now())
	if wait > 0 {
		if err := d.sleep(ctx, wait); err != nil {
			return fmt.Errorf("dataexport: waiting for circuit breaker: %w", err)
		}
	}
	attempts := d.cfg.MaxAttempts
	if trial {
		attempts = 1
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, d.cfg.SendTimeout)
		err := fn(sendCtx)
		cancel()
		if err == nil {
			d.br.success()
			d.st.recordSuccess(count, d.now())
			return nil
		}
		last = err
		d.st.recordFailure(err, d.now())
		d.br.failure(d.now())
		var perm *permanentError
		if errors.As(err, &perm) || ctx.Err() != nil || attempt == attempts {
			break
		}
		if serr := d.sleep(ctx, d.backoff(attempt)); serr != nil {
			break
		}
	}
	return last
}

// batcher drains a queue into batches and hands each to a deliverer.
type batcher[T any] struct {
	q    *queue[T]
	d    *deliverer
	send func(context.Context, []T) error
	size func(T) int
}

func (b *batcher[T]) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.q.notify:
		}
		for b.q.len() > 0 {
			if b.q.len() < b.d.cfg.BatchLines && b.d.cfg.FlushInterval > 0 {
				if err := b.d.sleep(ctx, b.d.cfg.FlushInterval); err != nil {
					return
				}
			}
			batch := b.q.take(b.d.cfg.BatchLines, b.d.cfg.BatchBytes, b.size)
			if len(batch) == 0 {
				break
			}
			if err := b.d.do(ctx, len(batch), func(c context.Context) error { return b.send(c, batch) }); err != nil {
				b.d.st.dropped.Add(int64(len(batch)))
			}
			if ctx.Err() != nil {
				return
			}
		}
	}
}
