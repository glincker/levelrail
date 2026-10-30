package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ExitRecorder persists a release container's exit and its last output.
// *store.DB satisfies it.
type ExitRecorder interface {
	RecordDeployExit(ctx context.Context, e store.DeployExit) error
}

// WithExitRecorder captures the last output of a release container that
// exits before it can serve. nil captures nothing.
func WithExitRecorder(r ExitRecorder) Option {
	return func(ctrl *Controller) { ctrl.exitRecorder = r }
}

// WithStartupWatch makes a release with no readiness probe and no stable
// window still prove it keeps running for d before it takes traffic.
func WithStartupWatch(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.startupWatch = d }
}

const (
	// ExitLogTailLines is how many of an exited container's last lines are kept.
	ExitLogTailLines    = 200
	exitLogReadBudget   = 5 * time.Second
	startupPollInterval = 250 * time.Millisecond
)

type containerLogReader interface {
	Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error)
}

// watchStartup is waitReady for a release with nothing to probe: without a
// stable window, a container that exits right away would otherwise count
// as ready the instant it started.
func (c *Controller) watchStartup(ctx context.Context, name string, desired *store.DesiredService) error {
	if c.safetyFor(ctx, desired).StableWindow > 0 {
		return nil
	}
	// Always one inspect: an instant exit must not pass with the watch off.
	if crash := c.exitedCrash(ctx, name); crash != nil {
		return crash
	}
	if c.startupWatch <= 0 {
		return nil
	}
	if _, ok := c.runtime.(docker.ExitStateInspector); !ok {
		return nil
	}
	deadline := time.Now().Add(c.startupWatch)
	for {
		if crash := c.exitedCrash(ctx, name); crash != nil {
			return crash
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("startup watch for %s: %w", name, ctx.Err())
		case <-time.After(min(startupPollInterval, left)):
		}
	}
}

// captureGateExit builds the stability gate's message for an exited
// container and captures it. With no readiness probe the stable window
// is the only readiness proof, so the exit reads as "before ready".
func (c *Controller) captureGateExit(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService, noReadiness bool, reason string, es docker.ExitState, obs deploysafety.GateObservation) string {
	if es == (docker.ExitState{}) {
		es = docker.ExitState{OOMKilled: obs.OOMKilled, ExitCode: obs.ExitCode, StartedAt: obs.StartedAt}
	}
	phase := deploysafety.ExitPhaseWindow
	if noReadiness {
		phase = deploysafety.ExitPhaseReadiness
	}
	v := deploysafety.ClassifyExit(deploysafety.WorkloadService, phase, exitObservation(es, time.Now()))
	v.Reason = reason
	c.captureExit(ctx, desired, state.Name, es, v)
	return v.Message
}

// captureExit records why name stopped and its last output. Best effort:
// the capture explains a failed deploy, it never decides one.
func (c *Controller) captureExit(ctx context.Context, desired *store.DesiredService, name string, es docker.ExitState, v deploysafety.ExitVerdict) {
	if c.exitRecorder == nil {
		return
	}
	finished := es.FinishedAt
	if finished.IsZero() && !es.StartedAt.IsZero() && v.Ran > 0 {
		finished = es.StartedAt.Add(v.Ran)
	}
	if err := c.exitRecorder.RecordDeployExit(ctx, store.DeployExit{
		ServiceName: c.serviceName, Image: desired.Image, Container: name,
		ExitCode: es.ExitCode, OOMKilled: es.OOMKilled, Reason: v.Reason, Message: v.Message,
		StartedAt: es.StartedAt, FinishedAt: finished, LogTail: c.tailContainerLogs(ctx, name),
		CapturedAt: time.Now(),
	}); err != nil {
		slog.Warn("reconcile: deploy exit evidence write failed, diagnostics are incomplete", slog.String("service", c.serviceName), slog.String("container", name), slog.String("error", err.Error()))
	}
}

// tailContainerLogs reads the last ExitLogTailLines of name, which works
// for a stopped container too. nil when the transport cannot read logs.
func (c *Controller) tailContainerLogs(ctx context.Context, name string) []store.DeployExitLine {
	reader, ok := c.runtime.(containerLogReader)
	if !ok {
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, exitLogReadBudget)
	defer cancel()
	lines, errs := reader.Logs(lctx, name, false, time.Time{})
	out := make([]store.DeployExitLine, 0, ExitLogTailLines)
	for l := range lines {
		out = append(out, store.DeployExitLine{Timestamp: l.Timestamp, Stream: l.Stream, Message: l.Message})
		if len(out) > ExitLogTailLines {
			out = out[1:]
		}
	}
	<-errs
	return out
}
