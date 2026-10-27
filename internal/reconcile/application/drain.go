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

// DrainTracker remembers each service's cutovers across passes, so each
// release drains from when traffic left it. In memory: after a restart a
// release's retirement falls back to its successor's creation, then the hold.
type DrainTracker struct {
	mu       sync.Mutex
	cutover  map[string]time.Time
	serving  map[string]string
	replaced map[string]string
	retired  map[string]map[string]time.Time
	shrink   map[string]bool
}

// NewDrainTracker builds an empty tracker.
func NewDrainTracker() *DrainTracker {
	return &DrainTracker{
		cutover: make(map[string]time.Time), serving: make(map[string]string),
		replaced: make(map[string]string), retired: make(map[string]map[string]time.Time),
		shrink: make(map[string]bool),
	}
}

// shrinking records whether service has surplus replicas of its serving
// release and reports when they first appear, which opens a drain window.
func (t *DrainTracker) shrinking(service string, surplus bool) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	opened := surplus && !t.shrink[service]
	t.shrink[service] = surplus
	return opened
}

// retire records when traffic left release; a release serving again is
// no longer retired.
func (t *DrainTracker) retire(service, release, serving string, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.retired[service]
	if m == nil {
		m = make(map[string]time.Time)
		t.retired[service] = m
	}
	delete(m, serving)
	if release != "" {
		m[release] = at
	}
}

// retiredAt is when traffic left release, if observed since start.
func (t *DrainTracker) retiredAt(service, release string) (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.retired[service][release]
	return at, ok
}

// forgetRetired drops retirements of releases no longer observed.
func (t *DrainTracker) forgetRetired(service string, present map[string]bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for rel := range t.retired[service] {
		if !present[rel] {
			delete(t.retired[service], rel)
		}
	}
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
		c.drain.retire(c.serviceName, c.drain.previous(c.serviceName), releaseOf(targets[0]), now)
	}
	// The scale-down check and the drain branch share one listing per pass.
	var listed []docker.ContainerState
	list := func() ([]docker.ContainerState, error) {
		if listed != nil {
			return listed, nil
		}
		all, err := c.listPrefix(ctx, c.serviceName+"-")
		if err != nil {
			return nil, fmt.Errorf("list containers for %s: %w", c.serviceName, err)
		}
		listed = all
		return all, nil
	}
	if !justDeployed && cfg.DrainTimeout > 0 && len(targets) > 0 {
		all, err := list()
		if err != nil {
			return err
		}
		// A scale-down changes no release, but the surplus replicas still
		// serve in-flight requests until a drain window passes.
		if c.drain.shrinking(c.serviceName, c.hasSurplusReplicas(all, targets)) {
			c.drain.mark(c.serviceName, now)
		}
	}
	if justDeployed {
		c.drain.mark(c.serviceName, now)
	}
	if c.draining(cfg, justDeployed, now) {
		all, err := list()
		if err != nil {
			return err
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
		all, err := list()
		if err != nil {
			return err
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

// hasSurplusReplicas reports whether a stale replica of the current release
// is still running, as after a scale-down.
func (c *Controller) hasSurplusReplicas(all []docker.ContainerState, targets []string) bool {
	current := releaseOf(targets[0])
	for _, cs := range c.filterStale(all, targets) {
		if cs.Running && releaseOf(cs.Name) == current {
			return true
		}
	}
	return false
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
	// Only a running release can be the one serving; a newer stopped one
	// (a failed deploy) must not date its retirement after a restart.
	newestStale := newestRunningRelease(stale, first, currentBase, serving)
	retired := map[string]time.Time{}
	present := map[string]bool{currentBase: true}
	for i, rel := range order {
		present[rel] = true
		if rel == currentBase {
			continue
		}
		at := cutover
		// Without an observed cutover, date it from the successor's creation;
		// an unknown creation keeps the release for the hold pass.
		if t, ok := c.drain.retiredAt(c.serviceName, rel); ok {
			at = t
		} else if rel != serving && rel != newestStale && i+1 < len(order) && !first[order[i+1]].IsZero() {
			at = first[order[i+1]]
		}
		retired[rel] = at
	}
	c.drain.forgetRetired(c.serviceName, present)
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
