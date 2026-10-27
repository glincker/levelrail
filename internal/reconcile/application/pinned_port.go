package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
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

// releasePinnedPort stops the running containers of this service that hold
// the pinned host port target needs. A pinned port cannot be bound twice,
// so blue-green overlap is impossible: the old release is stopped (not
// removed) so a failed deploy can restore it.
func (c *Controller) releasePinnedPort(ctx context.Context, target string, desired *store.DesiredService) ([]docker.ContainerState, error) {
	pinned := pinnedHostPorts(desired)
	if len(pinned) == 0 {
		return nil, nil
	}
	if until, ok := c.handoffFailed[target]; ok && time.Now().Before(until) {
		return nil, &pinnedPortBackoffError{port: pinned[0].HostPort, until: until}
	}
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return nil, fmt.Errorf("list containers holding pinned host port %s: %w", pinned[0], err)
	}
	var released []docker.ContainerState
	for _, cs := range all {
		if cs.Name == target || !cs.Running || !ownsContainer(c.serviceName, cs.Name) || !c.ownsInstance(cs) || !holdsAnyHostPort(cs, pinned) {
			continue
		}
		if err := c.stopContainer(ctx, cs.ID); err != nil {
			_ = c.restoreReleased(ctx, released)
			return nil, fmt.Errorf("stop %q to free its pinned host ports: %w", cs.Name, err)
		}
		released = append(released, cs)
	}
	return released, nil
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
		if state, err := c.runtime.InspectByName(ctx, target); err == nil && state != nil && state.Running {
			_ = c.stopContainer(ctx, state.ID)
		}
		return cause
	}
	if state, err := c.runtime.InspectByName(ctx, target); err == nil && state != nil {
		_ = c.stopContainer(ctx, state.ID)
		_ = c.removeContainer(ctx, state.ID)
	}
	if c.handoffFailed == nil {
		c.handoffFailed = map[string]time.Time{}
	}
	c.handoffFailed[target] = time.Now().Add(c.effectivePinnedPortRetry())
	if err := c.restoreReleased(ctx, released); err != nil {
		return fmt.Errorf("%w (and the previous release could not be restored: %w)", cause, err)
	}
	return fmt.Errorf("%w (previous release restored, retrying after %s)", cause, c.effectivePinnedPortRetry())
}

// requiredSecretError is a declared required secret with no stored value.
type requiredSecretError struct{ name string }

func (e *requiredSecretError) Error() string {
	return fmt.Sprintf("env var %q is required but no secret value has been set for it yet, set one and redeploy", e.name)
}

const reasonRequiredSecretMissing = "RequiredSecretMissing"

func createFailureReason(err error) string {
	var remote *publishedPortsRemoteError
	if errors.As(err, &remote) {
		return ReasonPublishedPortsRemoteNode
	}
	var missing *requiredSecretError
	if errors.As(err, &missing) {
		return reasonRequiredSecretMissing
	}
	var exhausted *docker.PoolExhaustedError
	if errors.As(err, &exhausted) {
		return reasonNetworkPoolExhausted
	}
	return "CreateFailed"
}

const reasonNetworkPoolExhausted = "NetworkPoolExhausted"
