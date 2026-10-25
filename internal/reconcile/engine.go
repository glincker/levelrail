// Package reconcile implements the core convergence loop: desired state
// in, observed state diffed against it, idempotent and level-triggered
// controllers converge the two. Reconcilers
// never assume they're continuing a previous partial operation: every
// call re-derives what to do from current observed state, which is what
// makes them safe to interrupt and safe to retry.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/footprint"
)

// Controller manages one resource's convergence from desired to observed
// state. Reconcile must be idempotent: calling it repeatedly with nothing
// changed must be a cheap no-op, and calling it after a previous call was
// interrupted mid-operation must still converge correctly rather than
// double-apply or corrupt state.
type Controller interface {
	// Name identifies the controller in logs and status lookups. Stable
	// across restarts.
	Name() string

	// Reconcile diffs desired against observed state and takes whatever
	// action is needed to converge them. It returns the conditions
	// resulting from this attempt even when it also returns an error;
	// callers should record both.
	Reconcile(ctx context.Context) (Result, error)
}

// Store is optional persistence for reconcile status. Every reconcile
// is required to emit a status condition with a reason string, stored
// and shown in the UI; without a Store, Engine still
// tracks the latest result via LastResult, but only in memory, lost on
// restart. internal/store.DB satisfies this interface structurally, no
// import of internal/store needed here (or that would be a cycle, since
// internal/store already imports this package for the Condition type).
type Store interface {
	UpsertConditions(ctx context.Context, controllerName string, conditions []Condition) error
}

// Engine runs a fixed set of controllers, triggered by Docker events with
// a periodic resync as a safety net for any event the stream missed. This
// push-primary/pull-as-safety-net shape is deliberate: Coolify's own v5
// rearchitecture converged on the same pattern independently.
type Engine struct {
	controllers []Controller
	source      Source
	logger      *slog.Logger
	store       Store // nil is valid: persistence is optional, not required

	mu         sync.RWMutex
	lastResult map[string]Result
	lastErr    map[string]error

	stats       passStats
	ring        []time.Duration
	slowest     time.Duration
	slowestName string

	passSlowest     time.Duration
	passSlowestName string

	// nudge lets a caller outside the reconcile loop (internal/api, on a
	// desired-state-changing request) request an immediate ReconcileAll
	// instead of waiting for the next Docker event or resync tick.
	// Buffered 1: a nudge is a level-triggered "something changed, look
	// again" signal, not a queue of individual requests, so any number
	// of Nudge calls between two Run passes collapses into the one
	// ReconcileAll that was already going to happen anyway.
	nudge chan struct{}

	started atomic.Bool

	guard *Guard

	drainTimeout time.Duration

	requeue requeueState

	persisted  persistLedger
	fullResync time.Duration
	now        func() time.Time
}

// Source dynamically supplies the current set of controllers to reconcile,
// called once per pass immediately before those controllers run. A Source
// re-derives its controller set from live desired state every call, the
// same level-triggered principle every controller in this codebase already
// follows for its own resource: nothing about which apps or databases
// exist is remembered between calls, everything is re-derived from what's
// in the store right now. This is how the engine goes from a fixed set of
// controllers built once at startup to one controller per app and per
// database, appearing and disappearing as desired state changes, without
// a restart.
type Source func(ctx context.Context) ([]Controller, error)

// NewEngine builds an Engine over the given controllers. Order is
// preserved for each ReconcileAll pass but controllers must not depend on
// each other's side effects within a single pass: that's what makes
// per-resource-type controllers safe to reason about independently.
func NewEngine(logger *slog.Logger, controllers ...Controller) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		controllers: controllers,
		logger:      logger,
		lastResult:  make(map[string]Result, len(controllers)),
		lastErr:     make(map[string]error, len(controllers)),
		nudge:       make(chan struct{}, 1),
		guard:       NewGuard(GuardConfigFromEnv(os.Getenv)),
		fullResync:  FullResyncFromEnv(os.Getenv),
		now:         time.Now,
	}
}

// SetFullResync overrides how long an unchanged status skips its database write. Call it before Run.
func (e *Engine) SetFullResync(d time.Duration) {
	if d > 0 {
		e.fullResync = d
	}
}

// Guard returns the storm protection state for the health page and its Fix.
func (e *Engine) Guard() *Guard { return e.guard }

// SetDrainTimeout lets a reconcile pass that is running when Run's context ends
// finish, for at most d, instead of being cut mid-operation. Zero keeps the
// old behavior of cancelling the pass with the context.
func (e *Engine) SetDrainTimeout(d time.Duration) { e.drainTimeout = d }

// pass runs one ReconcileAll that survives a shutdown signal for drainTimeout.
func (e *Engine) pass(ctx context.Context) {
	if e.drainTimeout <= 0 {
		e.ReconcileAll(ctx)
		return
	}
	pctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			t := time.NewTimer(e.drainTimeout)
			defer t.Stop()
			select {
			case <-done:
			case <-t.C:
				e.logger.Warn("reconcile pass cut at the shutdown drain limit", slog.Duration("limit", e.drainTimeout))
				cancel()
			}
		}
	}()
	e.ReconcileAll(pctx)
	close(done)
	cancel()
}

// SetGuard replaces the default storm protection. Call it before Run.
func (e *Engine) SetGuard(g *Guard) { e.guard = g }

// Started reports whether Run has begun; it stays true after Run returns.
func (e *Engine) Started() bool { return e.started.Load() }

// Nudge requests an immediate ReconcileAll pass on the next Run
// iteration, instead of waiting for the next Docker event or
// resyncInterval tick. Safe to call from any goroutine, including
// concurrently with Run and with other Nudge calls; a full buffer means
// a pass is already pending, so this never blocks. Reconcile's own
// idempotent, level-triggered contract is what makes an extra,
// unscheduled pass always safe to run.
func (e *Engine) Nudge() {
	select {
	case e.nudge <- struct{}{}:
	default:
	}
}

// NudgeController closes name's breaker and requests an immediate pass, for a
// change (such as a new probe) that makes retrying a failing controller worthwhile now.
func (e *Engine) NudgeController(name string) {
	e.guard.Reset(name)
	e.Nudge()
}

// SetStore attaches persistence for reconcile status. Call it once before
// Run, if at all; not safe to call concurrently with reconciles in flight.
func (e *Engine) SetStore(s Store) {
	e.store = s
}

// SetSource attaches a dynamic controller source, reconciled in addition
// to the fixed controllers NewEngine was built with. Call it once before
// Run, if at all; not safe to call concurrently with reconciles in flight.
func (e *Engine) SetSource(s Source) {
	e.source = s
}

// ReconcileAll runs every controller once: first the fixed set NewEngine
// was built with, then whatever the Source currently reports, if one is
// set. A single controller failing does not stop the others from running:
// one broken resource should never block convergence of everything else.
// A Source error is logged and skipped for this pass rather than treated
// as fatal, since the next tick or event re-derives the set again anyway.
func (e *Engine) ReconcileAll(ctx context.Context) {
	start := time.Now()
	n := len(e.controllers)
	defer func() { e.recordPass(start, n) }()
	live := make(map[string]struct{}, n)
	for _, c := range e.controllers {
		live[c.Name()] = struct{}{}
		e.reconcileOne(ctx, c)
	}
	if e.source == nil {
		return
	}
	dynamic, err := e.source(ctx)
	if err != nil {
		e.logger.Error("listing dynamic controllers failed", slog.String("error", err.Error()))
		return
	}
	n += len(dynamic)
	for _, c := range dynamic {
		live[c.Name()] = struct{}{}
		e.reconcileOne(ctx, c)
	}
	e.persisted.forget(live)
}

// safeReconcile turns a controller panic into an error so one broken resource cannot crash the process.
func (e *Engine) safeReconcile(ctx context.Context, c Controller) (result Result, panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("controller %s panicked: %v", c.Name(), r)
			e.logger.Error("reconcile panicked", slog.String("controller", c.Name()), slog.String("stack", string(debug.Stack())))
		}
	}()
	result, err = c.Reconcile(ctx)
	return result, false, err
}

func (e *Engine) reconcileOne(ctx context.Context, c Controller) {
	if !e.guard.Before(c.Name()) {
		return
	}
	start := time.Now()
	result, panicked, err := e.safeReconcile(ctx, c)
	e.guard.After(c.Name(), err, panicked)
	dur := time.Since(start)
	e.recordController(c.Name(), dur)

	e.mu.Lock()
	e.lastResult[c.Name()] = result
	e.lastErr[c.Name()] = err
	e.mu.Unlock()
	e.scheduleRequeue(c.Name(), result.RequeueAfter)

	// Persisting status is best-effort and secondary to the reconcile
	// itself: a database hiccup must never stop the controller from
	// converging the actual resource, so a store failure is logged, not
	// propagated as this reconcile's error.
	// An unchanged status is rewritten only when the full resync is due: the
	// row's updated_at then still proves the controller is being reconciled.
	changed := true
	if e.store != nil && len(result.Conditions) > 0 {
		now := e.now()
		sig, due := e.persisted.due(c.Name(), result.Conditions, now, e.fullResync)
		changed = due
		if due {
			if serr := e.store.UpsertConditions(ctx, c.Name(), result.Conditions); serr != nil {
				e.logger.Error("persisting reconcile status failed",
					slog.String("controller", c.Name()),
					slog.String("error", serr.Error()),
				)
			} else {
				e.persisted.record(c.Name(), sig, now)
			}
		}
	}

	attrs := []any{
		slog.String("controller", c.Name()),
		slog.Duration("duration", dur),
	}
	for _, cond := range result.Conditions {
		attrs = append(attrs,
			slog.Group(cond.Type,
				slog.String("status", string(cond.Status)),
				slog.String("reason", cond.Reason),
			),
		)
	}

	if err != nil {
		e.logger.Error("reconcile failed", append(attrs, slog.String("error", err.Error()))...)
		return
	}
	if changed {
		e.logger.Info("reconciled", attrs...)
		return
	}
	e.logger.Debug("reconciled", attrs...)
}

// LastResult returns the most recent Result and error for a controller by
// name, for status reporting. The zero Result and a nil error are
// returned if the controller has never run.
func (e *Engine) LastResult(name string) (Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastResult[name], e.lastErr[name]
}

// Run reconciles once immediately, then keeps reconciling on every
// incoming Docker event and on every resyncInterval tick, until ctx is
// cancelled or the event stream closes. Interrupting Run at any point is
// safe: nothing here holds a lock across a reconcile, so the next Run
// (or a fresh process) picks up wherever observed state actually is.
func (e *Engine) Run(ctx context.Context, events <-chan docker.Event, resyncInterval time.Duration) error {
	e.started.Store(true)
	e.logger.Info("reconcile engine starting",
		slog.Int("controllers", len(e.controllers)),
		slog.Duration("resync_interval", resyncInterval),
	)

	loop := footprint.Track("reconcile pass", resyncInterval)
	defer loop.Active()()
	pass := func() {
		start := time.Now()
		e.pass(ctx)
		loop.Observe(start, nil)
	}
	pass()

	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()

	// A burst of events past the pass budget collapses into one deferred pass: level-triggered, so nothing is lost.
	var deferTimer *time.Timer
	var deferC <-chan time.Time
	defer func() {
		if deferTimer != nil {
			deferTimer.Stop()
		}
	}()
	trigger := func() {
		ok, wait := e.guard.AllowPass()
		if !ok {
			if deferC == nil {
				deferTimer = time.NewTimer(wait)
				deferC = deferTimer.C
			}
			return
		}
		if deferC != nil {
			deferTimer.Stop()
			deferC = nil
		}
		pass()
	}

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("reconcile engine stopping", slog.String("reason", ctx.Err().Error()))
			return ctx.Err()

		case <-ticker.C:
			e.logger.Debug("resync tick")
			trigger()

		case <-deferC:
			deferC = nil
			trigger()

		case <-e.nudge:
			e.logger.Debug("nudge triggered reconcile")
			trigger()

		case ev, ok := <-events:
			if !ok {
				e.logger.Warn("docker event stream closed, continuing on resync ticker alone")
				events = nil // avoid a hot loop selecting on a closed, always-ready channel
				continue
			}
			e.logger.Debug("event triggered reconcile",
				slog.String("container", ev.ContainerName),
				slog.String("action", string(ev.Action)),
			)
			trigger()
		}
	}
}
