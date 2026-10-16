package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Environment overrides and defaults for build limits. Zero disables a limit.
const (
	EnvBuildTimeout       = "APP_BUILD_TIMEOUT"
	EnvBuildMaxConcurrent = "APP_BUILD_MAX_CONCURRENT"

	DefaultBuildTimeout       = 30 * time.Minute
	DefaultBuildMaxConcurrent = 2

	recordTimeout = 5 * time.Second
)

// ErrBuildTimeout is the cause of a build stopped by its time limit.
var ErrBuildTimeout = errors.New("build timed out")

// Limits caps builds. A zero field means unlimited.
type Limits struct {
	MaxConcurrent int
	Timeout       time.Duration
}

// LimitsFromEnv reads the env defaults; a malformed value falls back to the default.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	l := Limits{MaxConcurrent: DefaultBuildMaxConcurrent, Timeout: DefaultBuildTimeout}
	if v, ok := lookup(EnvBuildTimeout); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			l.Timeout = d
		}
	}
	if v, ok := lookup(EnvBuildMaxConcurrent); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			l.MaxConcurrent = n
		}
	}
	return l
}

// LimitsSource resolves the limits for the next build, so a settings change
// applies without a restart.
type LimitsSource func(ctx context.Context) Limits

// Clock lets tests drive the timeout and queue wait without sleeping.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// RunRecorder persists one finished build.
type RunRecorder interface {
	RecordBuildRun(ctx context.Context, r Run) error
}

// Builder is the build surface the Limiter wraps and itself provides.
type Builder interface {
	Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error)
	BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error)
}

// LimiterOption configures a Limiter.
type LimiterOption func(*Limiter)

// WithRunRecorder records every finished build.
func WithRunRecorder(r RunRecorder) LimiterOption { return func(l *Limiter) { l.rec = r } }

// WithLimiterClock overrides the clock, for tests.
func WithLimiterClock(c Clock) LimiterOption { return func(l *Limiter) { l.clock = c } }

// WithLimiterLogger overrides the logger.
func WithLimiterLogger(lg *slog.Logger) LimiterOption { return func(l *Limiter) { l.log = lg } }

// Limiter queues builds past the concurrency limit, stops builds past the
// time limit, and records each outcome with its cache hit counts.
type Limiter struct {
	inner  Builder
	limits LimitsSource
	rec    RunRecorder
	clock  Clock
	log    *slog.Logger

	mu      sync.Mutex
	running int
	limit   int
	waiters []chan struct{}
}

// NewLimiter wraps inner. limits is read once per build.
func NewLimiter(inner Builder, limits LimitsSource, opts ...LimiterOption) *Limiter {
	l := &Limiter{inner: inner, limits: limits, clock: realClock{}, log: slog.Default()}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Build implements the deploy pipeline's ImageBuilder.
func (l *Limiter) Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error) {
	return l.run(ctx, req.Tag, progress, func(c context.Context, p func(ProgressEvent)) (*Result, error) {
		return l.inner.Build(c, req, p)
	})
}

// BuildRailpack implements the deploy pipeline's ImageBuilder.
func (l *Limiter) BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error) {
	return l.run(ctx, req.Tag, progress, func(c context.Context, p func(ProgressEvent)) (*Result, error) {
		return l.inner.BuildRailpack(c, req, p)
	})
}

func (l *Limiter) run(ctx context.Context, tag string, progress func(ProgressEvent), do func(context.Context, func(ProgressEvent)) (*Result, error)) (*Result, error) {
	lim := l.limits(ctx)
	queued := l.clock.Now()
	if err := l.acquire(ctx, lim.MaxConcurrent); err != nil {
		return nil, fmt.Errorf("build: waiting for a build slot: %w", err)
	}
	defer l.release()

	started := l.clock.Now()
	run := Run{App: AppFromTag(tag), Tag: tag, StartedAt: started, QueueWait: started.Sub(queued)}

	bctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if lim.Timeout > 0 {
		timer := l.clock.After(lim.Timeout)
		go func() {
			select {
			case <-timer:
				cancel(ErrBuildTimeout)
			case <-bctx.Done():
			}
		}()
	}

	var steps stepCounter
	var smu sync.Mutex
	wrapped := func(ev ProgressEvent) {
		smu.Lock()
		steps.observe(ev)
		smu.Unlock()
		if progress != nil {
			progress(ev)
		}
	}

	res, err := do(bctx, wrapped)
	run.Duration = l.clock.Now().Sub(started)
	smu.Lock()
	run.CachedSteps, run.TotalSteps = steps.cached, steps.total()
	smu.Unlock()

	switch {
	case err == nil:
		run.Status = RunSuccess
	case errors.Is(context.Cause(bctx), ErrBuildTimeout):
		run.Status = RunTimeout
		err = fmt.Errorf("%w after %s: %w", ErrBuildTimeout, lim.Timeout, err)
	case ctx.Err() != nil:
		run.Status = RunCanceled
	default:
		run.Status = RunFailed
	}
	if err != nil {
		run.Error = err.Error()
	}
	l.record(ctx, run)
	return res, err
}

// record never fails a build: a finished build whose stats could not be saved
// is still a finished build.
func (l *Limiter) record(ctx context.Context, r Run) {
	if l.rec == nil {
		return
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := l.rec.RecordBuildRun(rctx, r); err != nil {
		l.log.Warn("build: record build run failed", slog.String("app", r.App), slog.String("error", err.Error()))
	}
}

func (l *Limiter) acquire(ctx context.Context, limit int) error {
	l.mu.Lock()
	l.limit = limit
	if limit <= 0 || (l.running < limit && len(l.waiters) == 0) {
		l.running++
		l.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	l.waiters = append(l.waiters, ch)
	l.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		l.mu.Lock()
		for i, w := range l.waiters {
			if w == ch {
				l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
				l.mu.Unlock()
				return ctx.Err()
			}
		}
		l.mu.Unlock()
		// Admitted at the same moment the context ended: hand the slot back.
		l.release()
		return ctx.Err()
	}
}

func (l *Limiter) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.running--
	for len(l.waiters) > 0 && (l.limit <= 0 || l.running < l.limit) {
		ch := l.waiters[0]
		l.waiters = l.waiters[1:]
		l.running++
		close(ch)
	}
}

// Queued reports builds waiting for a slot and builds running.
func (l *Limiter) Queued() (waiting, running int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiters), l.running
}
