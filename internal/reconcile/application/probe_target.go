package application

import (
	"context"
	"errors"
	"io"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/probe"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithProbeLimits sets redirect, exec-output and default-timing bounds for
// readiness and liveness probes. Defaults to probe's own defaults.
func WithProbeLimits(l probe.Limits) Option {
	return func(ctrl *Controller) { ctrl.probeLimits = l }
}

func (c *Controller) prober() *probe.Prober {
	return probe.New(c.httpClient, runtimeExecutor{runtime: c.runtime, outputCap: c.probeLimits.ExecOutputBytes}, c.probeLimits)
}

func readinessProbeFor(desired *store.DesiredService) *store.ServiceProbe {
	if desired.Health == nil {
		return nil
	}
	return runnableProbe(desired.Health.Readiness, desired.Port)
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

func probeTarget(state *docker.ContainerState, p store.ServiceProbe) (probe.Target, error) {
	if !p.NeedsPort() {
		return probe.Target{ContainerID: state.ID}, nil
	}
	addr, err := primaryAddr(state)
	if err != nil {
		return probe.Target{}, err
	}
	return probe.Target{Addr: addr}, nil
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
