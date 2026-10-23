package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DrainTracker remembers each service's last cutover across passes, so the
// drain is measured from cutover. In memory: a restart falls back to the hold.
type DrainTracker struct {
	mu      sync.Mutex
	cutover map[string]time.Time
}

// NewDrainTracker builds an empty tracker.
func NewDrainTracker() *DrainTracker {
	return &DrainTracker{cutover: make(map[string]time.Time)}
}

// WithDrainTracker measures the drain window from the real cutover time.
func WithDrainTracker(t *DrainTracker) Option {
	return func(ctrl *Controller) { ctrl.drain = t }
}

func (t *DrainTracker) mark(service string, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cutover[service] = at
}

func (t *DrainTracker) at(service string) time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cutover[service]
}

// effectiveHold is how long the previous release outlives a cutover: at
// least the drain timeout and, with auto-rollback, the watch window, so a
// watch-triggered rollback lands on a release that is still running.
func (c *Controller) effectiveHold(cfg deploysafety.Effective) time.Duration {
	hold := max(c.previousReleaseHold, cfg.DrainTimeout)
	if cfg.AutoRollback {
		hold = max(hold, cfg.WatchWindow)
	}
	return hold
}

// draining reports whether the previous release must keep serving because
// the cutover happened less than the drain timeout ago.
func (c *Controller) draining(cfg deploysafety.Effective, justDeployed bool, now time.Time) bool {
	if cfg.DrainTimeout <= 0 {
		return false
	}
	if justDeployed {
		return true
	}
	cut := c.drain.at(c.serviceName)
	return !cut.IsZero() && now.Before(cut.Add(cfg.DrainTimeout))
}

// retireStale removes stale containers after the cutover, keeping every
// running one while the drain window is open and the newest previous
// release for the hold after that.
func (c *Controller) retireStale(ctx context.Context, targets []string, desired *store.DesiredService, justDeployed bool) error {
	cfg := c.safetyFor(ctx, desired)
	now := time.Now()
	if justDeployed {
		c.drain.mark(c.serviceName, now)
	}
	if c.draining(cfg, justDeployed, now) {
		stale, err := c.staleContainers(ctx, targets)
		if err != nil {
			return err
		}
		var stopped []docker.ContainerState
		for _, cs := range stale {
			if !cs.Running {
				stopped = append(stopped, cs)
			}
		}
		if justDeployed && len(stale) > len(stopped) {
			c.recordGate(ctx, desired, "", deploysafety.PhaseDrain, deploysafety.StatusStarted, "",
				fmt.Sprintf("previous release keeps serving in-flight requests for %s", cfg.DrainTimeout))
		}
		return c.removeContainers(ctx, stopped)
	}

	hold := c.effectiveHold(cfg)
	var (
		due       []docker.ContainerState
		heldUntil time.Time
	)
	if hold <= 0 {
		stale, err := c.staleContainers(ctx, targets)
		if err != nil {
			return err
		}
		due = stale
	} else {
		all, err := c.listPrefix(ctx, c.serviceName+"-")
		if err != nil {
			return fmt.Errorf("list containers for %s: %w", c.serviceName, err)
		}
		due, heldUntil = c.heldFilterFor(all, targets, now, hold)
		c.recordHold(ctx, heldUntil)
	}
	running := 0
	for _, cs := range due {
		if cs.Running {
			running++
		}
	}
	err := c.removeContainers(ctx, due)
	if err == nil && running > 0 && cfg.DrainTimeout > 0 {
		c.recordGate(ctx, desired, "", deploysafety.PhaseDrain, deploysafety.StatusPassed, "",
			fmt.Sprintf("stopped %d container(s) of the previous release after draining", running))
	}
	return err
}
