package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// WithProbeLimits sets redirect, exec-output and default-timing bounds for
// readiness and liveness probes. Defaults to probe's own defaults.
func WithProbeLimits(l probe.Limits) Option {
	return func(ctrl *Controller) { ctrl.probeLimits = l }
}

// prober builds the Prober every readiness/liveness check uses. opts is
// almost always empty; deployProber is the one caller that passes
// probe.WithOnAttempt, scoped to deploy-time readiness probing alone.
func (c *Controller) prober(opts ...probe.Option) *probe.Prober {
	return probe.New(c.httpClient, runtimeExecutor{runtime: c.runtime, outputCap: c.probeLimits.ExecOutputBytes}, c.probeLimits, opts...)
}

func readinessProbeFor(desired *store.DesiredService) *store.ServiceProbe {
	if desired.Health == nil {
		return nil
	}
	return runnableProbe(desired.Health.Readiness, desired.Port)
}

// deployReadinessProbeFor is readinessProbeFor plus an implicit TCP connect
// to the declared port, so a release that never listens cannot be cut over.
// Deploy-time only, and only with the deploy-safety gate wired: re-checking
// already-running replicas keeps the explicit probe.
func (c *Controller) deployReadinessProbeFor(desired *store.DesiredService) *store.ServiceProbe {
	if p := readinessProbeFor(desired); p != nil {
		return p
	}
	if c.gateDefaults != nil && desired.Port > 0 {
		return &store.ServiceProbe{TCP: true}
	}
	return nil
}

func livenessProbeFor(desired *store.DesiredService) *store.ServiceProbe {
	if desired.Health == nil {
		return nil
	}
	return runnableProbe(desired.Health.Liveness, desired.Port)
}

// runnableProbe drops an HTTP probe on a service with no port: there is
// nothing to connect to, so it is treated as unconfigured, as it always was.
func runnableProbe(p *store.ServiceProbe, port int) *store.ServiceProbe {
	if p == nil || (p.NeedsPort() && port == 0) {
		return nil
	}
	return p
}

// probeTarget resolves what a probe dials. A port probe on a remote node
// dials its mesh address: the control plane's loopback is the wrong host.
func (c *Controller) probeTarget(ctx context.Context, state *docker.ContainerState, p store.ServiceProbe, nodeID string) (probe.Target, error) {
	if !p.NeedsPort() {
		return probe.Target{ContainerID: state.ID}, nil
	}
	addr, err := PrimaryAddr(state)
	if err != nil {
		return probe.Target{}, err
	}
	if c.meshPaths == nil || nodeID == "" {
		return probe.Target{Addr: addr}, nil
	}
	path, err := c.meshPaths.Path(ctx, nodeID)
	if err != nil {
		return probe.Target{}, fmt.Errorf("resolve mesh path to node %s: %w", nodeID, err)
	}
	// No mesh for this node: the agent shares the control plane's network.
	if path.Local || path.NoMesh {
		return probe.Target{Addr: addr}, nil
	}
	if !path.Usable {
		return probe.Target{}, fmt.Errorf("node %s is not reachable over the mesh: %s", nodeID, path.Reason)
	}
	bound := state.Ports[0].HostIP
	if ip := net.ParseIP(bound); ip != nil && ip.IsLoopback() {
		return probe.Target{}, fmt.Errorf("port is published on loopback of node %s, waiting for the mesh bind", nodeID)
	}
	if bound != "" && bound != "0.0.0.0" && bound != path.Address {
		return probe.Target{Addr: net.JoinHostPort(bound, strconv.Itoa(state.Ports[0].HostPort))}, nil
	}
	return probe.Target{Addr: net.JoinHostPort(path.Address, strconv.Itoa(state.Ports[0].HostPort))}, nil
}

// runtimeExecutor adapts docker.Runtime's streaming Exec to probe.Executor.
type runtimeExecutor struct {
	runtime   docker.Runtime
	outputCap int
}

func (e runtimeExecutor) ExecProbe(ctx context.Context, containerID string, cmd []string) (int, string, error) {
	rc, err := e.runtime.Exec(ctx, containerID, cmd)
	if err != nil {
		return 0, "", err
	}
	limit := e.outputCap
	if limit <= 0 {
		limit = probe.DefaultExecOutputBytes
	}

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		buf := &cappedBuffer{limit: limit + 1}
		_, readErr := io.Copy(buf, rc)
		done <- result{out: buf.String(), err: readErr}
	}()

	select {
	case <-ctx.Done():
		// Closing the stream unblocks the copy; Docker has no API to kill
		// an exec'd process, so a hung command keeps running in the container.
		_ = rc.Close()
		return 0, "", ctx.Err()
	case r := <-done:
		_ = rc.Close()
		var exitErr *docker.ExecExitError
		switch {
		case r.err == nil:
			return 0, r.out, nil
		case errors.As(r.err, &exitErr):
			out := r.out
			if exitErr.Stderr != "" {
				out = joinOutput(out, exitErr.Stderr)
			}
			return exitErr.ExitCode, out, nil
		default:
			return 0, r.out, r.err
		}
	}
}

func joinOutput(stdout, stderr string) string {
	if stdout == "" {
		return stderr
	}
	return stdout + "\n" + stderr
}
