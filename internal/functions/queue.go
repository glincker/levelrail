package functions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Headers sent on every queued call so the handler can correlate and deduplicate it.
const (
	HeaderInvocation = "X-Function-Invocation"
	HeaderAttempt    = "X-Function-Attempt"
	HeaderSource     = "X-Function-Source"
)

// QueueStore is the persistence the worker needs.
type QueueStore interface {
	ClaimFunctionInvocation(ctx context.Context, now time.Time) (store.FunctionInvocation, bool, error)
	SettleFunctionInvocation(ctx context.Context, id, status string, next time.Time, code int, errMsg string, now time.Time) error
	RequeueRunningFunctionInvocations(ctx context.Context, now time.Time) (int64, error)
	PruneFunctionInvocations(ctx context.Context, cutoff time.Time) (int64, error)
}

// QueueApps resolves a function to its service.
type QueueApps interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// Target wakes a function and returns the base URL it answers on.
type Target interface {
	Prepare(ctx context.Context, svc *store.DesiredService) (string, error)
}

// Queue delivers queued calls to sleeping functions with retries and a dead-letter state.
type Queue struct {
	Store  QueueStore
	Apps   QueueApps
	Target Target
	Config TriggerConfig
	Logger *slog.Logger
	Client *http.Client
	Now    func() time.Time
	wake   chan struct{}
}

var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (q *Queue) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}

func (q *Queue) client() *http.Client {
	if q.Client != nil {
		return q.Client
	}
	return noRedirectClient
}

// Kick asks idle workers to look for work now instead of at the next poll.
func (q *Queue) Kick() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Run starts the worker pool and blocks until ctx ends.
func (q *Queue) Run(ctx context.Context) error {
	q.wake = make(chan struct{}, 1)
	if n, err := q.Store.RequeueRunningFunctionInvocations(ctx, q.now()); err != nil {
		return fmt.Errorf("functions: recover queue: %w", err)
	} else if n > 0 {
		q.Logger.Info("functions: requeued interrupted invocations", slog.Int64("count", n))
	}
	var wg sync.WaitGroup
	for i := 0; i < max(q.Config.Workers, 1); i++ {
		wg.Add(1)
		go func(first bool) {
			defer wg.Done()
			q.work(ctx, first)
		}(i == 0)
	}
	wg.Wait()
	return ctx.Err()
}

func (q *Queue) work(ctx context.Context, janitor bool) {
	tick := time.NewTicker(q.Config.PollInterval)
	defer tick.Stop()
	for {
		for q.step(ctx) {
		}
		if janitor {
			if _, err := q.Store.PruneFunctionInvocations(ctx, q.now().Add(-q.Config.DoneRetention)); err != nil && ctx.Err() == nil {
				q.Logger.Warn("functions: prune finished invocations failed", slog.String("error", err.Error()))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-q.wake:
		}
	}
}

// step claims and delivers one call; it reports whether there may be more work.
func (q *Queue) step(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	inv, ok, err := q.Store.ClaimFunctionInvocation(ctx, q.now())
	if err != nil {
		if ctx.Err() == nil {
			q.Logger.Error("functions: claim invocation failed", slog.String("error", err.Error()))
		}
		return false
	}
	if !ok {
		return false
	}
	q.deliver(ctx, inv)
	return true
}

func (q *Queue) deliver(ctx context.Context, inv store.FunctionInvocation) {
	log := q.Logger.With(slog.String("invocation_id", inv.ID), slog.String("function", inv.Function), slog.Int("attempt", inv.Attempts))
	code, retryable, err := q.attempt(ctx, inv)
	now := q.now()
	switch {
	case err == nil:
		q.settle(ctx, log, inv.ID, store.FunctionInvocationDone, now, code, "", now)
	case !retryable || inv.Attempts >= inv.MaxAttempts:
		log.Warn("functions: invocation moved to dead letters", slog.String("error", err.Error()), slog.Int("status", code))
		q.settle(ctx, log, inv.ID, store.FunctionInvocationDead, now, code, err.Error(), now)
	default:
		next := now.Add(q.Config.Backoff(inv.Attempts))
		log.Info("functions: invocation will retry", slog.String("error", err.Error()), slog.Time("next_attempt_at", next))
		q.settle(ctx, log, inv.ID, store.FunctionInvocationQueued, next, code, err.Error(), now)
	}
}

func (q *Queue) settle(ctx context.Context, log *slog.Logger, id, status string, next time.Time, code int, msg string, now time.Time) {
	sctx := context.WithoutCancel(ctx)
	if err := q.Store.SettleFunctionInvocation(sctx, id, status, next, code, msg, now); err != nil {
		log.Error("functions: settle invocation failed", slog.String("error", err.Error()))
	}
}

// attempt makes one call and reports the status code, whether a failure is worth retrying, and the failure.
func (q *Queue) attempt(ctx context.Context, inv store.FunctionInvocation) (int, bool, error) {
	svc, err := q.Apps.GetDesiredService(ctx, inv.Function)
	if err != nil || svc == nil {
		return 0, false, fmt.Errorf("function %q not found", inv.Function)
	}
	actx, cancel := context.WithTimeout(ctx, q.Config.AttemptTimeout)
	defer cancel()
	base, err := q.Target.Prepare(actx, svc)
	if err != nil {
		return 0, true, fmt.Errorf("wake function: %w", err)
	}
	req, err := http.NewRequestWithContext(actx, inv.Method, base+inv.Path, bytes.NewReader(inv.Body))
	if err != nil {
		return 0, false, fmt.Errorf("build request: %w", err)
	}
	if inv.ContentType != "" {
		req.Header.Set("Content-Type", inv.ContentType)
	}
	req.Header.Set(HeaderInvocation, inv.ID)
	req.Header.Set(HeaderAttempt, fmt.Sprint(inv.Attempts))
	req.Header.Set(HeaderSource, inv.Source)
	resp, err := q.client().Do(req) //nolint:gosec // G704: the target is the operator's own function
	if err != nil {
		return 0, true, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, false, nil
	}
	retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests
	return resp.StatusCode, retryable, fmt.Errorf("function answered HTTP %d", resp.StatusCode)
}
