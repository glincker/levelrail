package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/probe"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Applied when the spec leaves a liveness field unset. The interval
// matches the reconcile engine's own resync cadence, since a
// level-triggered probe can never run more often than the loop it runs
// in; the threshold matches Kubernetes' own default.
const (
	defaultLivenessInterval = 30 * time.Second
	defaultLivenessFailures = 3
)

// livenessStopTimeout is how long a container gets to exit on its own
// before Docker kills it during a liveness-triggered restart, the same
// grace removeContainers already gives a stale container.
const livenessStopTimeout = 10 * time.Second

// livenessSeverity orders one pass's outcomes so a multi-replica service
// reports the worst thing that happened rather than the last.
type livenessSeverity int

const (
	livenessOK livenessSeverity = iota
	livenessProbeUnavailable
	livenessDegraded
	livenessRestarted
	livenessRestartFailed
)

// livenessOutcome is one replica's liveness result, or the worst of a
// whole pass's. err carries the detail the reported condition's Message
// shows, and is nil only when severity is livenessOK.
type livenessOutcome struct {
	severity livenessSeverity
	err      error
}

// livenessEntry is one container's probe history.
type livenessEntry struct {
	failures  int
	lastProbe time.Time
}

// LivenessTracker counts consecutive liveness failures per container,
// in memory only and never persisted. Persisting it would make the
// check depend on state carried across an interruption, which this
// codebase's reconcilers are not allowed to do (every pass re-derives
// what to do from observed state). The documented tradeoff: restarting
// the control plane resets every counter, so an already-failing app can
// take up to one extra threshold's worth of probes before it is
// restarted.
//
// Keyed by container name, which already carries the service name, so
// one tracker is safely shared by every service's controller. It has to
// be: the controller set is rebuilt from desired state on every
// reconcile pass (cmd/levelrail's dynamicSource), so a controller's own
// state never survives the pass that created it. See WithLivenessTracker.
type LivenessTracker struct {
	mu    sync.Mutex
	state map[string]*livenessEntry
}

// NewLivenessTracker builds an empty tracker. Long-running callers want
// exactly one, shared across every controller they build.
func NewLivenessTracker() *LivenessTracker {
	return &LivenessTracker{state: make(map[string]*livenessEntry)}
}

// due reports whether name is ready for its next probe. This is the
// only rate limit on probing: Reconcile runs on every Docker event, far
// more often than any sane probe interval.
func (t *LivenessTracker) due(name string, now time.Time, interval time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.state[name]
	if !ok {
		return true
	}
	return now.Sub(entry.lastProbe) >= interval
}

func (t *LivenessTracker) recordSuccess(name string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entry(name)
	entry.failures = 0
	entry.lastProbe = now
}

// recordFailure counts one failed probe and returns the resulting
// consecutive-failure count.
func (t *LivenessTracker) recordFailure(name string, now time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entry(name)
	entry.failures++
	entry.lastProbe = now
	return entry.failures
}

// reset clears name's failure history and restarts its probe interval,
// so a container that just (re)started is never judged by the failures
// of the instance it replaced.
func (t *LivenessTracker) reset(name string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state[name] = &livenessEntry{lastProbe: now}
}

// resetAll is reset for every name in names, plus retain: the shape a
// pass that actually deployed something needs.
func (t *LivenessTracker) resetAll(serviceName string, names []string, now time.Time) {
	t.retain(serviceName, names)
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, name := range names {
		t.state[name] = &livenessEntry{lastProbe: now}
	}
}

// retain drops serviceName's history for every container not in names,
// so an image change (which renames every target) cannot leave dead
// counters behind. Scoped to serviceName's own containers because one
// tracker is shared by every service's controller.
func (t *LivenessTracker) retain(serviceName string, names []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	keep := make(map[string]bool, len(names))
	for _, name := range names {
		keep[name] = true
	}
	for name := range t.state {
		if !keep[name] && ownsContainer(serviceName, name) {
			delete(t.state, name)
		}
	}
}

// entry returns name's history, creating it if needed. Callers hold mu.
func (t *LivenessTracker) entry(name string) *livenessEntry {
	if existing, ok := t.state[name]; ok {
		return existing
	}
	entry := &livenessEntry{}
	t.state[name] = entry
	return entry
}

// checkLiveness probes every target due for a probe and restarts any
// container whose consecutive failures reach the spec'd threshold. It
// is what makes a hung-but-running container visible: readiness only
// ever runs during a deploy's cutover, so without this a container that
// deadlocks after passing readiness is never checked again.
//
// Fully opt-in, exactly like readiness: a service with no
// health.liveness block, or no port to probe, is never probed at all.
func (c *Controller) checkLiveness(ctx context.Context, targets []string, desired *store.DesiredService) livenessOutcome {
	cfg := livenessProbeFor(desired)
	if cfg == nil {
		c.liveness.retain(c.serviceName, nil)
		return livenessOutcome{}
	}
	c.liveness.retain(c.serviceName, targets)

	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultLivenessInterval
	}
	threshold := cfg.Failures
	if threshold <= 0 {
		threshold = defaultLivenessFailures
	}

	now := time.Now()
	worst := livenessOutcome{}
	for _, target := range targets {
		out := c.checkReplicaLiveness(ctx, target, *cfg, interval, threshold, now)
		if out.severity > worst.severity {
			worst = out
		}
	}
	return worst
}

func livenessProbeFor(desired *store.DesiredService) *store.ServiceProbe {
	if desired.Health == nil || desired.Health.Liveness == nil || desired.Port == 0 {
		return nil
	}
	return desired.Health.Liveness
}

func (c *Controller) checkReplicaLiveness(ctx context.Context, target string, cfg store.ServiceProbe, interval time.Duration, threshold int, now time.Time) livenessOutcome {
	if !c.liveness.due(target, now, interval) {
		return livenessOutcome{}
	}

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return livenessOutcome{severity: livenessProbeUnavailable, err: fmt.Errorf("liveness probe: inspect %q: %w", target, err)}
	}
	if state == nil || !state.Running {
		// ensureReplicaRunning already owns a missing or stopped
		// container; probing one would only bank a meaningless failure
		// against the instance that replaces it.
		c.liveness.reset(target, now)
		return livenessOutcome{}
	}

	addr, err := primaryAddr(state)
	if err != nil {
		return livenessOutcome{severity: livenessProbeUnavailable, err: fmt.Errorf("liveness probe: %w", err)}
	}

	probeErr := probe.Check(ctx, c.httpClient, addr, probe.Config{Path: cfg.Path, Timeout: cfg.Timeout})
	if probeErr == nil {
		c.liveness.recordSuccess(target, now)
		return livenessOutcome{}
	}
	return c.handleLivenessFailure(ctx, target, state, threshold, now, probeErr)
}

// handleLivenessFailure counts one failure and, once threshold
// consecutive failures are in, restarts the container.
func (c *Controller) handleLivenessFailure(ctx context.Context, target string, state *docker.ContainerState, threshold int, now time.Time, probeErr error) livenessOutcome {
	failures := c.liveness.recordFailure(target, now)
	if failures < threshold {
		return livenessOutcome{severity: livenessDegraded, err: fmt.Errorf("liveness probe for %q failed (%d/%d): %w", target, failures, threshold, probeErr)}
	}

	if err := c.restartReplica(ctx, state); err != nil {
		return livenessOutcome{severity: livenessRestartFailed, err: fmt.Errorf("liveness probe for %q failed %d times and the restart failed: %w", target, failures, err)}
	}
	c.liveness.reset(target, now)
	return livenessOutcome{severity: livenessRestarted, err: fmt.Errorf("liveness probe for %q failed %d times (%v), container restarted", target, failures, probeErr)}
}

// restartReplica stops and starts the container in place, so its name
// (and with it every ContainerName-based lookup, ingress included) is
// unchanged. A failure leaves it stopped, which the next pass's
// ensureReplicaRunning already knows how to recover: start it, or
// recreate it if Start keeps failing.
func (c *Controller) restartReplica(ctx context.Context, state *docker.ContainerState) error {
	if err := c.runtime.Stop(ctx, state.ID, livenessStopTimeout); err != nil {
		return fmt.Errorf("stop %q: %w", state.Name, err)
	}
	if err := c.runtime.Start(ctx, state.ID); err != nil {
		return fmt.Errorf("start %q: %w", state.Name, err)
	}
	return nil
}

// steadyStateResult is the tail every strategy takes on a pass that
// deployed nothing: the replica set is already converged, so the
// liveness check is the only thing left to run.
func (c *Controller) steadyStateResult(ctx context.Context, targets []string, desired *store.DesiredService) (reconcile.Result, error) {
	out := c.checkLiveness(ctx, targets, desired)
	switch out.severity {
	case livenessRestarted:
		return notReady("LivenessFailedRestarting", out.err), fmt.Errorf("application/%s: %w", c.serviceName, out.err)
	case livenessRestartFailed:
		return notReady("LivenessRestartFailed", out.err), fmt.Errorf("application/%s: %w", c.serviceName, out.err)
	case livenessDegraded:
		// Still serving and still under the threshold, which is the
		// whole point of having a threshold: one blip must not flip the
		// app to "attention needed" in the dashboard.
		return readyWithDetail("LivenessDegraded", out.err), nil
	case livenessProbeUnavailable:
		// No evidence the app is unhealthy, only that this pass could
		// not find out, so the error surfaces without downgrading
		// Status, the same shape RunningStaleCleanupFailed already has.
		return readyWithDetail("LivenessProbeUnavailable", out.err), fmt.Errorf("application/%s: %w", c.serviceName, out.err)
	default:
		return ready("AlreadyRunning"), nil
	}
}
