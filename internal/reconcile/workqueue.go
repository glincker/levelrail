package reconcile

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

// EnvWorkers is how many keyed controllers may reconcile at once. 1 keeps the
// sequential pass exactly as it was; 4 or 8 is recommended for many apps.
const EnvWorkers = "APP_RECONCILE_WORKERS"

// EnvKeyTimeout bounds one keyed reconcile when EnvWorkers is above 1.
const EnvKeyTimeout = "APP_RECONCILE_KEY_TIMEOUT"

const (
	defaultWorkers    = 1
	maxWorkers        = 64
	defaultKeyTimeout = 10 * time.Minute
)

// WorkersFromEnv reads EnvWorkers; invalid or below 1 means 1.
func WorkersFromEnv(getenv func(string) string) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	n, err := strconv.Atoi(getenv(EnvWorkers))
	if err != nil || n < 1 {
		return defaultWorkers
	}
	return min(n, maxWorkers)
}

// KeyTimeoutFromEnv reads EnvKeyTimeout; invalid or non-positive means the default.
func KeyTimeoutFromEnv(getenv func(string) string) time.Duration {
	if getenv == nil {
		getenv = os.Getenv
	}
	if d, err := time.ParseDuration(getenv(EnvKeyTimeout)); err == nil && d > 0 {
		return d
	}
	return defaultKeyTimeout
}

type queued struct {
	c     Controller
	since time.Time
}

// keyQueue runs controllers on a bounded pool with at most one in flight per
// name. A name added again while queued keeps its place and takes the newer
// controller; added while in flight, it runs once more after the current run.
type keyQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  map[string]queued
	order    []string
	inflight map[string]context.CancelFunc
	closed   bool
	alive    int

	base    context.Context
	cancel  context.CancelFunc
	timeout time.Duration
	run     func(ctx context.Context, c Controller)
	wg      sync.WaitGroup
}

func newKeyQueue(parent context.Context, workers int, timeout time.Duration, run func(context.Context, Controller)) *keyQueue {
	base, cancel := context.WithCancel(context.WithoutCancel(parent))
	q := &keyQueue{
		pending:  map[string]queued{},
		inflight: map[string]context.CancelFunc{},
		base:     base,
		cancel:   cancel,
		timeout:  timeout,
		run:      run,
	}
	q.cond = sync.NewCond(&q.mu)
	q.alive = workers
	q.wg.Add(workers)
	for range workers {
		go q.worker()
	}
	return q
}

func (q *keyQueue) add(c Controller, now time.Time) {
	name := c.Name()
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	if p, ok := q.pending[name]; ok {
		q.pending[name] = queued{c: c, since: p.since}
		return
	}
	q.pending[name] = queued{c: c, since: now}
	q.order = append(q.order, name)
	q.cond.Signal()
}

// retain drops queued names absent from live and cancels their in-flight run,
// so a deleted app never keeps converging after its desired state is gone.
func (q *keyQueue) retain(live map[string]struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.order[:0]
	for _, name := range q.order {
		if _, ok := live[name]; ok {
			kept = append(kept, name)
			continue
		}
		delete(q.pending, name)
	}
	q.order = kept
	for name, cancel := range q.inflight {
		if _, ok := live[name]; !ok {
			cancel()
		}
	}
}

// next blocks until a name not already in flight is queued, FIFO by first add.
func (q *keyQueue) next() (string, Controller, context.Context, context.CancelFunc, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.closed {
			return "", nil, nil, nil, false
		}
		for i, name := range q.order {
			if _, busy := q.inflight[name]; busy {
				continue
			}
			c := q.pending[name].c
			delete(q.pending, name)
			q.order = append(q.order[:i], q.order[i+1:]...)
			ctx, cancel := context.WithTimeout(q.base, q.timeout)
			q.inflight[name] = cancel
			return name, c, ctx, cancel, true
		}
		q.cond.Wait()
	}
}

func (q *keyQueue) worker() {
	defer func() {
		q.mu.Lock()
		q.alive--
		q.mu.Unlock()
		q.wg.Done()
	}()
	for {
		name, c, ctx, cancel, ok := q.next()
		if !ok {
			return
		}
		q.run(ctx, c)
		cancel()
		q.mu.Lock()
		delete(q.inflight, name)
		q.mu.Unlock()
		// A name skipped while in flight may now be runnable by any worker.
		q.cond.Broadcast()
	}
}

// close stops taking work, lets in-flight runs finish for at most drain, then
// cancels them and waits for every worker to exit.
func (q *keyQueue) close(drain time.Duration) {
	q.mu.Lock()
	q.closed = true
	q.pending = map[string]queued{}
	q.order = nil
	q.cond.Broadcast()
	q.mu.Unlock()
	if drain <= 0 {
		q.cancel()
	}
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	if drain > 0 {
		t := time.NewTimer(drain)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
			q.cancel()
		}
	}
	<-done
	q.cancel()
}

// QueueStats is the keyed pool's state for the control plane health page.
type QueueStats struct {
	Workers    int           `json:"workers"`
	Depth      int           `json:"queue_depth"`
	InFlight   int           `json:"in_flight"`
	OldestWait time.Duration `json:"oldest_wait_ns"`
}

func (q *keyQueue) stats(now time.Time) QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := QueueStats{Workers: q.alive, Depth: len(q.order), InFlight: len(q.inflight)}
	for _, p := range q.pending {
		if w := now.Sub(p.since); w > s.OldestWait {
			s.OldestWait = w
		}
	}
	return s
}
