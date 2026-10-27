package application

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DrainTracker remembers each service's last cutover across passes, so the
// drain is measured from cutover. In memory: a restart falls back to the hold.
type DrainTracker struct {
	mu       sync.Mutex
	cutover  map[string]time.Time
	serving  map[string]string
	replaced map[string]string
}

// NewDrainTracker builds an empty tracker.
func NewDrainTracker() *DrainTracker {
	return &DrainTracker{cutover: make(map[string]time.Time), serving: make(map[string]string), replaced: make(map[string]string)}
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

// moved records release as service's serving release and reports whether it
// replaced a different one, which is a cutover even when nothing was created.
func (t *DrainTracker) moved(service, release string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prev := t.serving[service]
	t.serving[service] = release
	if prev == "" || prev == release {
		return false
	}
	t.replaced[service] = prev
	return true
}

// previous is the release that served service until its last observed
// cutover, or "" when none was observed since start.
func (t *DrainTracker) previous(service string) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.replaced[service]
}

func (t *DrainTracker) at(service string) time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cutover[service]
}

func (c *Controller) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
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
	now := c.clock()
	// A rollback onto a held release that is still running creates nothing,
	// but the release it replaces still owes its in-flight requests a drain.
	if len(targets) > 0 && c.drain.moved(c.serviceName, releaseOf(targets[0])) {
		justDeployed = true
	}
	if justDeployed {
		c.drain.mark(c.serviceName, now)
	}
	if c.draining(cfg, justDeployed, now) {
		all, err := c.listPrefix(ctx, c.serviceName+"-")
		if err != nil {
			return fmt.Errorf("list containers for %s: %w", c.serviceName, err)
		}
		cut := c.drain.at(c.serviceName)
		if cut.IsZero() {
			cut = now
		}
		due, draining := c.drainFilter(all, targets, cut, cfg.DrainTimeout, now)
		if justDeployed && draining {
			c.recordGate(ctx, desired, "", deploysafety.PhaseDrain, deploysafety.StatusStarted, "",
				fmt.Sprintf("previous release keeps serving in-flight requests for %s", cfg.DrainTimeout))
		}
		return c.removeContainers(ctx, due)
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

// drainFilter returns the stale containers due at now. Each release drains
// from the moment the next one took over, so overlapping cutovers each get a
// full window while long-retired releases (after a rollback) still go.
func (c *Controller) drainFilter(all []docker.ContainerState, targets []string, cutover time.Time, timeout time.Duration, now time.Time) (due []docker.ContainerState, draining bool) {
	stale := c.filterStale(all, targets)
	first, _ := releaseCreation(c.serviceName, all)
	currentBase := ""
	if len(targets) > 0 {
		currentBase = releaseOf(targets[0])
	}
	serving := c.drain.previous(c.serviceName)
	var rels []string
	seen := map[string]bool{}
	for _, cs := range stale {
		if rel := releaseOf(cs.Name); !seen[rel] && rel != currentBase {
			seen[rel] = true
			rels = append(rels, rel)
		}
	}
	order := append([]string{currentBase}, rels...)
	sort.SliceStable(order, func(i, j int) bool { return releaseNewer(order[j], order[i], first, serving) })
	newestStale := ""
	for _, rel := range rels {
		if newestStale == "" || releaseNewer(rel, newestStale, first, serving) {
			newestStale = rel
		}
	}
	retired := map[string]time.Time{}
	for i, rel := range order {
		if rel == currentBase {
			continue
		}
		at := cutover
		// An unknown successor creation time keeps the release: the hold pass cleans it up.
		if rel != serving && rel != newestStale && i+1 < len(order) && !first[order[i+1]].IsZero() {
			at = first[order[i+1]]
		}
		retired[rel] = at
	}
	for _, cs := range stale {
		rel := releaseOf(cs.Name)
		if cs.Running && (rel == currentBase || now.Before(retired[rel].Add(timeout))) {
			draining = true
			continue
		}
		due = append(due, cs)
	}
	return due, draining
}
