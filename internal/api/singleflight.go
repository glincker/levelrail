package api

import (
	"context"
	"sync"
	"time"
)

// flightEntry is one in-flight or cached computation for a key.
type flightEntry struct {
	done    chan struct{}
	val     any
	err     error
	expires time.Time
}

// flightGroup collapses concurrent callers of the same key into one run, caches the
// result for the live TTL and caps how many distinct keys compute at once.
type flightGroup struct {
	mu      sync.Mutex
	entries map[string]*flightEntry
	sem     chan struct{}
}

func newFlightGroup(maxConcurrent int) *flightGroup {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &flightGroup{entries: map[string]*flightEntry{}, sem: make(chan struct{}, maxConcurrent)}
}

// do returns the cached or shared result for key, running fn at most once per ttl window.
func (g *flightGroup) do(ctx context.Context, key string, ttl time.Duration, fn func(context.Context) (any, error)) (any, error) {
	g.mu.Lock()
	if e, ok := g.entries[key]; ok {
		select {
		case <-e.done:
			if e.err == nil && time.Now().Before(e.expires) {
				g.mu.Unlock()
				return e.val, nil
			}
		default:
			g.mu.Unlock()
			return waitFlight(ctx, e)
		}
	}
	e := &flightEntry{done: make(chan struct{})}
	g.entries[key] = e
	g.mu.Unlock()

	// The run must outlive the first caller, so later waiters still get a result.
	runCtx := context.WithoutCancel(ctx)
	go func() {
		defer close(e.done)
		g.sem <- struct{}{}
		defer func() { <-g.sem }()
		e.val, e.err = fn(runCtx)
		e.expires = time.Now().Add(ttl)
		if e.err != nil {
			g.mu.Lock()
			delete(g.entries, key)
			g.mu.Unlock()
		}
	}()
	return waitFlight(ctx, e)
}

func waitFlight(ctx context.Context, e *flightEntry) (any, error) {
	select {
	case <-e.done:
		return e.val, e.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// forget removes a finished entry; a running one is left so its waiters still get the result.
func (g *flightGroup) forget(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.entries[key]; ok {
		select {
		case <-e.done:
			delete(g.entries, key)
		default:
		}
	}
}
