// Package reconcile implements the core convergence loop described in
// CLAUDE.md 4.2: desired state in, observed state diffed against it,
// idempotent and level-triggered controllers converge the two. Reconcilers
// never assume they're continuing a previous partial operation — every
// call re-derives what to do from current observed state, which is what
// makes them safe to interrupt and safe to retry.
package reconcile

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
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
	// resulting from this attempt even when it also returns an error —
	// callers should record both.
	Reconcile(ctx context.Context) (Result, error)
}

// Engine runs a fixed set of controllers, triggered by Docker events with
// a periodic resync as a safety net for any event the stream missed. This
// push-primary/pull-as-safety-net shape is deliberate — Coolify's own v5
// rearchitecture (see docs-local/research/prior-art-coolify.md) converged
// on the same pattern independently.
type Engine struct {
	controllers []Controller
	logger      *slog.Logger

	mu         sync.RWMutex
	lastResult map[string]Result
	lastErr    map[string]error
}

// NewEngine builds an Engine over the given controllers. Order is
// preserved for each ReconcileAll pass but controllers must not depend on
// each other's side effects within a single pass — that's what makes
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
	}
}

// ReconcileAll runs every controller once, in order. A single controller
// failing does not stop the others from running — one broken resource
// should never block convergence of everything else.
func (e *Engine) ReconcileAll(ctx context.Context) {
	for _, c := range e.controllers {
		e.reconcileOne(ctx, c)
	}
}

func (e *Engine) reconcileOne(ctx context.Context, c Controller) {
	start := time.Now()
	result, err := c.Reconcile(ctx)
	dur := time.Since(start)

	e.mu.Lock()
	e.lastResult[c.Name()] = result
	e.lastErr[c.Name()] = err
	e.mu.Unlock()

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
	e.logger.Info("reconciled", attrs...)
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
// safe — nothing here holds a lock across a reconcile, so the next Run
// (or a fresh process) picks up wherever observed state actually is.
func (e *Engine) Run(ctx context.Context, events <-chan docker.Event, resyncInterval time.Duration) error {
	e.logger.Info("reconcile engine starting",
		slog.Int("controllers", len(e.controllers)),
		slog.Duration("resync_interval", resyncInterval),
	)

	e.ReconcileAll(ctx)

	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("reconcile engine stopping", slog.String("reason", ctx.Err().Error()))
			return ctx.Err()

		case <-ticker.C:
			e.logger.Debug("resync tick")
			e.ReconcileAll(ctx)

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
			e.ReconcileAll(ctx)
		}
	}
}
