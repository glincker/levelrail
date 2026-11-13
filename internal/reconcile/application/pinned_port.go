package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultPinnedPortRetry  = 5 * time.Minute
	reasonPinnedPortWait    = "PinnedPortHandoffBackoff"
	reasonPinnedPortUnbound = "PinnedPortNotPublished"
)

// pinnedPortBackoffError means a failed handoff is still inside its retry
// delay, so the previous release was left serving.
type pinnedPortBackoffError struct {
	port  int
	until time.Time
}

func (e *pinnedPortBackoffError) Error() string {
	if e.port == 0 {
		return fmt.Sprintf("the last deploy failed while taking over from the running release, which was restored and keeps serving; next attempt after %s",
			e.until.UTC().Format(time.RFC3339))
	}
	return fmt.Sprintf("the last deploy failed while taking over pinned host port %d, the previous release was restored and keeps serving; next attempt after %s",
		e.port, e.until.UTC().Format(time.RFC3339))
}

// WithPinnedPortRetry sets how long a failed pinned-port handoff waits
// before the next attempt. Every attempt stops the serving release, so
// retrying a broken image each pass would repeat the outage.
func WithPinnedPortRetry(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.pinnedPortRetry = d }
}

func (c *Controller) effectivePinnedPortRetry() time.Duration {
	if c.pinnedPortRetry > 0 {
		return c.pinnedPortRetry
	}
	return defaultPinnedPortRetry
}

// pinnedPortUnbound reports a running container Docker started without
// publishing the pinned host port, which a failed first start can leave
// behind. Nothing listens on the port, yet the container looks healthy.
func pinnedPortUnbound(state *docker.ContainerState, desired *store.DesiredService) bool {
	if desired.HostPort == nil || desired.Port == 0 || state == nil || !state.Running {
		return false
	}
	for _, p := range state.Ports {
		if p.HostPort == *desired.HostPort && bindingProtocol(p) == store.PortProtocolTCP {
			return false
		}
	}
	return true
}

// handoffPlan is what a takeover must free before the next release starts.
type handoffPlan struct {
	pinned    []store.PublishedPort
	streams   map[int]bool
	exclusive bool
}

func (h handoffPlan) stopFirst() bool {
	return len(h.pinned) > 0 || len(h.streams) > 0 || h.exclusive
}

func (c *Controller) planHandoff(ctx context.Context, desired *store.DesiredService) handoffPlan {
	pinned := pinnedHostPorts(desired)
	// A stream listener binds its host port in ingress, so a previous
	// release still publishing it must stop first.
	streams, _ := c.streamHostPorts(ctx, desired.Name)
	return handoffPlan{pinned: pinned, streams: streams, exclusive: holdsExclusiveData(desired, pinned, streams)}
}

// rolloutBudgetError means an explicit rollout budget forbids taking the
// only replica down, so the serving release was left untouched.
type rolloutBudgetError struct{ floor int }

func (e *rolloutBudgetError) Error() string {
	return fmt.Sprintf("the release holds a host port or single-writer data, so a deploy must stop it first, but the app's rollout budget keeps at least %d replica(s) available; relax max_unavailable or min_available to allow the short outage", e.floor)
}

const reasonRolloutBudget = "RolloutBudgetForbidsStopFirst"

// stopFirstFloor is the replicas an explicit per-app rollout policy keeps
// available. Env defaults never count: they are not a promise made for
// this app.
func (c *Controller) stopFirstFloor(ctx context.Context) int {
	src, ok := c.store.(RolloutPolicySource)
	if !ok {
		return 0
	}
	p, found, err := src.GetRolloutPolicy(ctx, c.serviceName)
	if err != nil || !found {
		return 0
	}
	pol := rolloutpolicy.Policy{MaxSurge: p.MaxSurge, MaxUnavailable: p.MaxUnavailable, MinAvailable: p.MinAvailable}
	res := rolloutpolicy.Resolve(pol, rolloutpolicy.OSEnv, 1)
	floor := 0
	if pol.MinAvailable != "" {
		floor = res.MinAvailable
	}
	if pol.MaxUnavailable != "" {
		floor = max(floor, 1-res.MaxUnavailable)
	}
	return floor
}

func (c *Controller) handoffBlockedUntil(target string) (time.Time, bool) {
	c.handoffMu.Lock()
	defer c.handoffMu.Unlock()
	until, ok := c.handoffFailed[target]
	return until, ok && time.Now().Before(until)
}

func (c *Controller) noteHandoffFailed(target string) {
	c.handoffMu.Lock()
	defer c.handoffMu.Unlock()
	if c.handoffFailed == nil {
		c.handoffFailed = map[string]time.Time{}
	}
	c.handoffFailed[target] = time.Now().Add(c.effectivePinnedPortRetry())
}

func (c *Controller) clearHandoffFailed(target string) {
	c.handoffMu.Lock()
	defer c.handoffMu.Unlock()
	delete(c.handoffFailed, target)
}

// releasePinnedPort stops the running containers of this service that hold
// the pinned host port target needs. A pinned port cannot be bound twice,
// so blue-green overlap is impossible: the old release is stopped (not
// removed) so a failed deploy can restore it.
func (c *Controller) releasePinnedPort(ctx context.Context, target string, desired *store.DesiredService) ([]docker.ContainerState, error) {
	plan := c.planHandoff(ctx, desired)
	if !plan.stopFirst() {
		return nil, nil
	}
	if until, blocked := c.handoffBlockedUntil(target); blocked {
		port := 0
		if len(plan.pinned) > 0 {
			port = plan.pinned[0].HostPort
		}
		return nil, &pinnedPortBackoffError{port: port, until: until}
	}
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return nil, fmt.Errorf("list containers holding the host ports or data of %s: %w", c.serviceName, err)
	}
	var toStop []docker.ContainerState
	for _, cs := range all {
		if cs.Name == target || !cs.Running || !ownsContainer(c.serviceName, cs.Name) || !c.ownsInstance(cs) {
			continue
		}
		if !plan.exclusive && !holdsAnyHostPort(cs, plan.pinned) && !holdsStreamHostPort(cs, plan.streams) {
			continue
		}
		toStop = append(toStop, cs)
	}
	if len(toStop) == 0 {
		return nil, nil
	}
	if floor := c.stopFirstFloor(ctx); floor > 0 {
		return nil, &rolloutBudgetError{floor: floor}
	}
	if plan.exclusive {
		// Fail on a broken volume or missing host path before the serving release is stopped, not after.
		for _, v := range desired.Volumes {
			if err := docker.EnsureVolumeFor(ctx, c.runtime, v.Name, volumeOwner(desired, v.Name)); err != nil {
				return nil, fmt.Errorf("ensure volume %q: %w", v.Name, err)
			}
		}
		if err := c.checkBindSources(desired); err != nil {
			return nil, err
		}
	}
	var released []docker.ContainerState
	for _, cs := range toStop {
		if err := c.stopContainer(ctx, cs.ID); err != nil {
			_ = c.restoreReleased(ctx, released)
			return nil, fmt.Errorf("stop %q to free its host ports or data volume: %w", cs.Name, err)
		}
		released = append(released, cs)
	}
	return released, nil
}

// checkBindSources fails before a stop when a writable bind source is
// missing on this node, as a missing volume would. Remote nodes are not
// visible from here and are left to the create step.
func (c *Controller) checkBindSources(desired *store.DesiredService) error {
	local := ""
	if c.meshDB != nil {
		local = c.meshDB.LocalNodeID
	}
	if !SameNode(desired.NodeID, "", local) {
		return nil
	}
	for _, m := range desired.BindMounts {
		if m.ReadOnly || socketPath(m.HostPath) {
			continue
		}
		if _, err := os.Stat(m.HostPath); err != nil {
			return fmt.Errorf("bind mount source %q is not usable, the previous release was left running: %w", m.HostPath, err)
		}
	}
	return nil
}

// socketPath reports a unix socket bind, which shares a daemon rather than data.
func socketPath(p string) bool {
	return strings.HasSuffix(path.Clean(p), ".sock")
}

// writableData reports persistent storage a second writer could corrupt:
// a named volume, or a writable bind that is not a socket. Read-only
// binds (config, certs) are safe to share.
func writableData(desired *store.DesiredService) bool {
	if len(desired.Volumes) > 0 {
		return true
	}
	for _, m := range desired.BindMounts {
		if !m.ReadOnly && !socketPath(m.HostPath) {
			return true
		}
	}
	return false
}

// holdsExclusiveData is the stop-first rule for single-writer data: one
// replica with writable storage AND a host-level claim (a fixed host port or
// a stream port) that already forces a single running release. A volume
// alone keeps the overlap; databases that lock a volume use
// strategy: recreate.
func holdsExclusiveData(desired *store.DesiredService, pinned []store.PublishedPort, streamPorts map[int]bool) bool {
	if desired.Replicas > 1 || !writableData(desired) {
		return false
	}
	for _, p := range pinned {
		if p.HostPort > 0 {
			return true
		}
	}
	return len(streamPorts) > 0
}

func holdsStreamHostPort(cs docker.ContainerState, ports map[int]bool) bool {
	for _, p := range cs.Ports {
		if ports[p.HostPort] && bindingProtocol(p) == store.PortProtocolTCP {
			return true
		}
	}
	return false
}

// pinnedHostPorts lists every fixed host port desired binds: the pinned
// main tcp port plus each published port. None can be bound twice.
func pinnedHostPorts(desired *store.DesiredService) []store.PublishedPort {
	var out []store.PublishedPort
	if desired.HostPort != nil && desired.Port != 0 {
		out = append(out, store.PublishedPort{ContainerPort: desired.Port, HostPort: *desired.HostPort, Protocol: store.PortProtocolTCP})
	}
	return append(out, desired.PublishedPorts...)
}

func holdsAnyHostPort(cs docker.ContainerState, pinned []store.PublishedPort) bool {
	for _, p := range cs.Ports {
		for _, want := range pinned {
			if p.HostPort == want.HostPort && bindingProtocol(p) == want.EffectiveProtocol() {
				return true
			}
		}
	}
	return false
}

func bindingProtocol(p docker.PortBinding) string {
	if p.Protocol == "" {
		return store.PortProtocolTCP
	}
	return p.Protocol
}

func holdsHostPort(cs docker.ContainerState, port int) bool {
	for _, p := range cs.Ports {
		if p.HostPort == port {
			return true
		}
	}
	return false
}

// restoreReleased starts released again, returning the first failure: a
// previous release that cannot come back must reach the operator.
func (c *Controller) restoreReleased(ctx context.Context, released []docker.ContainerState) error {
	var first error
	for _, cs := range released {
		if err := c.startContainer(ctx, cs.ID); err != nil && first == nil {
			first = fmt.Errorf("restart previous release %q: %w", cs.Name, err)
		}
	}
	return first
}

// abortHandoff undoes a failed pinned-port takeover: the new container is
// removed (it holds the port), the stopped releases are started again and
// the next attempt is delayed. cause is returned, joined with any restore
// failure.
func (c *Controller) abortHandoff(ctx context.Context, target string, released []docker.ContainerState, cause error) error {
	if len(released) == 0 {
		// Stopped, not removed: a failed candidate must not stay Up, and the
		// exited container keeps its output for the log collector.
		if state, err := c.runtime.InspectByName(ctx, target); err == nil && state != nil {
			c.parkFailedCandidate(ctx, state)
		}
		return cause
	}
	if state, err := c.runtime.InspectByName(ctx, target); err == nil && state != nil {
		_ = c.stopContainer(ctx, state.ID)
		_ = c.removeContainer(ctx, state.ID)
	}
	c.noteHandoffFailed(target)
	if err := c.restoreReleased(ctx, released); err != nil {
		return fmt.Errorf("%w (and the previous release could not be restored: %w)", cause, err)
	}
	return fmt.Errorf("%w (previous release restored, retrying after %s)", cause, c.effectivePinnedPortRetry())
}

// parkFailedCandidate stops a failed candidate and turns its Docker restart policy off, so a
// policy of unless-stopped or always cannot bring it back between passes or after a daemon restart.
func (c *Controller) parkFailedCandidate(ctx context.Context, state *docker.ContainerState) {
	if syncer, ok := c.runtime.(docker.RestartPolicySyncer); ok {
		_, _ = syncer.SyncRestartPolicy(ctx, state.ID, "no")
	}
	_ = c.stopContainer(ctx, state.ID)
}

// dockerRestartLoop quarantines a running container that Docker's own restart policy keeps cycling,
// stopping it and turning that policy off so the daemon cannot bring it back.
func (c *Controller) dockerRestartLoop(ctx context.Context, name string, state *docker.ContainerState) (bool, int) {
	inspector, ok := c.runtime.(docker.ExitStateInspector)
	if !ok {
		return false, 0
	}
	es, err := inspector.InspectExitState(ctx, name)
	if err != nil || es == nil {
		return false, 0
	}
	if !c.restartBackoff.ObserveDockerRestarts(name, es.RestartCount, time.Now()) {
		return false, 0
	}
	c.parkFailedCandidate(ctx, state)
	_, restarts := c.restartBackoff.Quarantined(name)
	return true, restarts
}

// requiredSecretError is a declared required secret with no stored value.
type requiredSecretError struct{ name string }

func (e *requiredSecretError) Error() string {
	return fmt.Sprintf("env var %q is required but no secret value has been set for it yet, set one and redeploy", e.name)
}

const reasonRequiredSecretMissing = "RequiredSecretMissing"

// reasonSecretResolveFailed marks a create blocked because an external secret
// reference could not be read. The wrapped error names the reference and
// connection, never a value.
const reasonSecretResolveFailed = "SecretResolveFailed"

type secretResolveError struct{ err error }

func (e *secretResolveError) Error() string { return e.err.Error() }
func (e *secretResolveError) Unwrap() error { return e.err }

func createFailureReason(err error) string {
	var remote *publishedPortsRemoteError
	if errors.As(err, &remote) {
		return ReasonPublishedPortsRemoteNode
	}
	var missing *requiredSecretError
	if errors.As(err, &missing) {
		return reasonRequiredSecretMissing
	}
	var unresolved *secretResolveError
	if errors.As(err, &unresolved) {
		return reasonSecretResolveFailed
	}
	var exhausted *docker.PoolExhaustedError
	if errors.As(err, &exhausted) {
		return reasonNetworkPoolExhausted
	}
	return "CreateFailed"
}

const reasonNetworkPoolExhausted = "NetworkPoolExhausted"
