package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvReadinessRequeue is the fixed delay between readiness probes of a
// candidate whose wait left the worker (only with more than one worker).
const EnvReadinessRequeue = "APP_RECONCILE_READINESS_REQUEUE"

const (
	defaultReadinessRequeue = 2 * time.Second
	reasonReadinessPending  = "ReadinessPending"
)

// ReadinessRequeueFromEnv reads EnvReadinessRequeue; unset or invalid means the default.
func ReadinessRequeueFromEnv(getenv func(string) string) time.Duration {
	if getenv == nil {
		getenv = os.Getenv
	}
	raw := getenv(EnvReadinessRequeue)
	if raw == "" {
		return defaultReadinessRequeue
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("reconcile: invalid readiness requeue delay, using default", slog.String("env", EnvReadinessRequeue), slog.String("value", raw), slog.String("default", defaultReadinessRequeue.String()))
		return defaultReadinessRequeue
	}
	return d
}

// pendingCandidate is a started candidate still waiting for readiness.
type pendingCandidate struct {
	containerID string
	created     bool
	released    []string // IDs of the previous release stopped for a pinned port
	snap        *store.AppliedConfig
	firstSeen   time.Time
}

// PendingReadiness remembers candidates between runs, since a controller is
// rebuilt every pass. Everything it holds is re-checked against live containers.
type PendingReadiness struct {
	mu    sync.Mutex
	boot  time.Time
	byKey map[string]map[string]pendingCandidate // service, then target
}

// NewPendingReadiness builds an empty tracker; containers started before it
// are treated as waits a control plane restart interrupted.
func NewPendingReadiness() *PendingReadiness {
	return &PendingReadiness{boot: time.Now(), byKey: map[string]map[string]pendingCandidate{}}
}

func (p *PendingReadiness) get(service, target string) (pendingCandidate, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.byKey[service][target]
	return c, ok
}

func (p *PendingReadiness) put(service, target string, c pendingCandidate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byKey[service] == nil {
		p.byKey[service] = map[string]pendingCandidate{}
	}
	p.byKey[service][target] = c
}

func (p *PendingReadiness) take(service, target string) (pendingCandidate, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.byKey[service][target]
	delete(p.byKey[service], target)
	if len(p.byKey[service]) == 0 {
		delete(p.byKey, service)
	}
	return c, ok
}

func (p *PendingReadiness) targets(service string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.byKey[service]))
	for t := range p.byKey[service] {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// WithReadinessRequeue moves the blue-green readiness wait off the worker:
// each run probes once and asks to be re-run after delay. nil keeps the blocking wait.
func WithReadinessRequeue(p *PendingReadiness, delay time.Duration) Option {
	return func(c *Controller) { c.pendingReady, c.readinessRequeue = p, delay }
}

// defersReadiness reports whether this run probes once instead of blocking.
// A release with nothing to probe keeps its short startup watch.
func (c *Controller) defersReadiness(desired *store.DesiredService) bool {
	return c.deferReadiness && !desired.RunsToCompletion() && c.deployReadinessProbeFor(desired) != nil
}

// startedAt is when name last started, from the runtime when it reports it.
func (c *Controller) startedAt(ctx context.Context, state *docker.ContainerState, fallback time.Time) time.Time {
	if inspector, ok := c.runtime.(docker.ExitStateInspector); ok {
		if es, err := inspector.InspectExitState(ctx, state.Name); err == nil && es != nil && !es.StartedAt.IsZero() {
			return es.StartedAt
		}
	}
	if !state.Created.IsZero() {
		return state.Created
	}
	return fallback
}

// probeCandidate is one readiness attempt bounded by a deadline derived from
// the container's start, so a restarted control plane resumes the same wait.
// pending true means not ready yet and still inside the budget.
func (c *Controller) probeCandidate(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService, started time.Time) (pending bool, err error) {
	readiness := c.deployReadinessProbeFor(desired)
	target, err := c.probeTarget(ctx, state, *readiness, desired.Port, desired.NodeID)
	if err != nil {
		if crash := c.exitedCrash(ctx, state.Name); crash != nil {
			return false, crash
		}
		return false, fmt.Errorf("readiness probe: %w", err)
	}
	cfg := c.wakeProbeConfig(ctx, desired, readiness.ProbeConfig())
	probeErr := c.deployProber(ctx, desired).Check(ctx, target, cfg)
	if probeErr == nil {
		return false, nil
	}
	if crash := c.exitedCrash(ctx, state.Name); crash != nil {
		return false, crash
	}
	budget := c.effectiveReadyBudget(desired)
	if time.Since(started) >= budget {
		return false, fmt.Errorf("readiness probe: never became ready within %s, last attempt: %w", budget, probeErr)
	}
	return true, nil
}

// deferCandidate records target as pending and returns the outcome that ends this run.
func (c *Controller) deferCandidate(target string, state *docker.ContainerState, base pendingCandidate) replicaOutcome {
	c.unconfirmedMu.Lock()
	if snap := c.unconfirmed[target]; snap != nil {
		base.snap = snap
	}
	delete(c.unconfirmed, target)
	c.unconfirmedMu.Unlock()
	base.containerID = state.ID
	c.pendingReady.put(c.serviceName, target, base)
	// The re-run reuses this controller, so its next listing must be live.
	c.invalidateSnapshot()
	return replicaOutcome{reason: reasonReadinessPending, pending: true}
}

// trackedCandidate returns the pending record this process stored for the
// container now named target, dropping one left by a replaced container.
func (c *Controller) trackedCandidate(ctx context.Context, target string, state *docker.ContainerState) (pendingCandidate, time.Time, bool) {
	p, ok := c.pendingReady.get(c.serviceName, target)
	if ok && p.containerID != state.ID {
		c.pendingReady.take(c.serviceName, target)
		ok = false
	}
	if !ok {
		return pendingCandidate{}, time.Time{}, false
	}
	return p, c.startedAt(ctx, state, p.firstSeen), true
}

// adoptable reports an unready running target whose wait a control plane
// restart interrupted: it started before this process, is inside its budget
// and another release of the service is still around to fall back to.
func (c *Controller) adoptable(ctx context.Context, target string, state *docker.ContainerState, desired *store.DesiredService) (time.Time, bool) {
	started := c.startedAt(ctx, state, time.Time{})
	if started.IsZero() || !started.Before(c.pendingReady.boot) || time.Since(started) >= c.effectiveReadyBudget(desired) {
		return time.Time{}, false
	}
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return time.Time{}, false
	}
	for _, cs := range all {
		if ownsContainer(c.serviceName, cs.Name) && c.ownsInstance(cs) && releaseOf(cs.Name) != releaseOf(target) {
			return started, true
		}
	}
	return time.Time{}, false
}

// resumeCandidate re-probes a pending candidate once and either defers it
// again or hands the verdict to the same path a blocking wait ends in.
func (c *Controller) resumeCandidate(ctx context.Context, target string, state *docker.ContainerState, desired *store.DesiredService, p pendingCandidate, started time.Time) (replicaOutcome, error) {
	pending, err := c.probeCandidate(ctx, state, desired, started)
	if pending {
		return c.deferCandidate(target, state, p), nil
	}
	c.pendingReady.take(c.serviceName, target)
	c.holdApplied(target, p.snap)
	return c.readinessVerdict(ctx, target, state, desired, p.created, c.locateReleased(ctx, p.released), err)
}

// stoppedCandidate fails a pending candidate that exited between runs the
// way a crash inside the blocking wait fails it, instead of crash-restarting it.
func (c *Controller) stoppedCandidate(ctx context.Context, target string, state *docker.ContainerState, desired *store.DesiredService) (replicaOutcome, bool, error) {
	p, ok := c.pendingReady.get(c.serviceName, target)
	if !ok || p.containerID != state.ID {
		return replicaOutcome{}, false, nil
	}
	c.pendingReady.take(c.serviceName, target)
	c.holdApplied(target, p.snap)
	cause := fmt.Errorf("readiness probe: %q stopped before it passed", target)
	if crash := c.exitedCrash(ctx, target); crash != nil {
		cause = crash
	}
	out, err := c.readinessVerdict(ctx, target, state, desired, p.created, c.locateReleased(ctx, p.released), cause)
	return out, true, err
}

// locateReleased finds the stopped previous release by ID among the
// service's live containers, skipping any that vanished or were restarted.
func (c *Controller) locateReleased(ctx context.Context, ids []string) []docker.ContainerState {
	if len(ids) == 0 {
		return nil
	}
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return nil
	}
	var out []docker.ContainerState
	for _, cs := range all {
		if !cs.Running && slices.Contains(ids, cs.ID) {
			out = append(out, cs)
		}
	}
	return out
}

func releasedIDs(released []docker.ContainerState) []string {
	ids := make([]string, 0, len(released))
	for _, cs := range released {
		ids = append(ids, cs.ID)
	}
	return ids
}

// abortSupersededPending stops pending candidates the current desired state
// no longer targets and puts back their stopped previous release, before a
// new candidate takes the pinned port.
func (c *Controller) abortSupersededPending(ctx context.Context, targets []string) {
	for _, t := range c.pendingReady.targets(c.serviceName) {
		if slices.Contains(targets, t) {
			continue
		}
		p, _ := c.pendingReady.take(c.serviceName, t)
		_ = c.abortSuperseded(ctx, t, c.locateReleased(ctx, p.released))
	}
}

// pendingResult is Ready False with no error, so the breaker does not count
// a candidate that is still inside its readiness budget.
func (c *Controller) pendingResult(target string) reconcile.Result {
	return reconcile.Result{
		Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse,
			Reason: reasonReadinessPending, Message: fmt.Sprintf("waiting for %s to pass its readiness probe", target),
		}},
		RequeueAfter: c.readinessRequeue,
		KeyOnly:      true,
	}
}
