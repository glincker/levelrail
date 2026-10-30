package application

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// GateRecorder persists a release's gate timeline and the reason it failed.
// *store.DB satisfies it.
type GateRecorder interface {
	RecordGateEvent(ctx context.Context, e store.DeployGateEvent) error
	MarkRolloutFailureReason(ctx context.Context, serviceName, image, reason string) error
}

// CutoverChecker tells whether a recent deploy of an image still lacks its
// cutover event. *store.DB satisfies it.
type CutoverChecker interface {
	DeployMissingCutover(ctx context.Context, serviceName, image string, since time.Time) (bool, error)
}

// recordAdoptedCutover covers a deploy whose target was already running (an
// instant rollback to the live or held release): no container is created,
// so without this the post-deploy watch never arms. Reading the store keeps
// it restart safe and tolerates the attempt row landing after the first pass.
func (c *Controller) recordAdoptedCutover(ctx context.Context, container string, desired *store.DesiredService) {
	checker, ok := c.gateRecorder.(CutoverChecker)
	if !ok {
		return
	}
	window := c.safetyFor(ctx, desired).WatchWindow
	if window <= 0 {
		return
	}
	missing, err := checker.DeployMissingCutover(ctx, c.serviceName, desired.Image, time.Now().Add(-window))
	if err != nil || !missing {
		return
	}
	c.recordGate(ctx, desired, container, deploysafety.PhaseCutover, deploysafety.StatusPassed, "",
		"traffic is on the release that was already running")
}

// DeploySafetySource returns an app's gate overrides. *store.DB satisfies it.
type DeploySafetySource interface {
	GetDeploySafetySettings(ctx context.Context, serviceName string) (store.DeploySafetySettings, error)
}

// WithStabilityGate holds a new container back from traffic until it passes
// N readiness checks in a row and a stable window with no restart or OOM kill.
func WithStabilityGate(defaults deploysafety.Effective, src DeploySafetySource) Option {
	return func(ctrl *Controller) {
		d := defaults
		ctrl.gateDefaults = &d
		ctrl.gateSource = src
	}
}

// WithGateRecorder records gate timeline events. nil records nothing.
func WithGateRecorder(r GateRecorder) Option {
	return func(ctrl *Controller) { ctrl.gateRecorder = r }
}

// WithGateTick overrides how often the stability gate re-checks a
// container; tests use it to avoid real multi-second waits.
func WithGateTick(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.gateTick = d }
}

const defaultGateTick = time.Second

// gateError is a stability gate failure with its shared reason constant.
type gateError struct {
	reason, message string
}

func (e *gateError) Error() string { return e.message }

// safetyFor resolves desired's effective gate settings; with no gate
// configured it is a single readiness pass and no window.
func (c *Controller) safetyFor(ctx context.Context, desired *store.DesiredService) deploysafety.Effective {
	if c.gateDefaults == nil {
		return deploysafety.Effective{SuccessThreshold: 1}
	}
	if c.gateSource == nil {
		return *c.gateDefaults
	}
	overrides, err := c.gateSource.GetDeploySafetySettings(ctx, desired.Name)
	if err != nil {
		return *c.gateDefaults
	}
	return deploysafety.Resolve(*c.gateDefaults, overrides)
}

func (c *Controller) recordGate(ctx context.Context, desired *store.DesiredService, container, phase, status, reason, message string) {
	c.traceGate(ctx, desired, container, phase, status, reason, message)
	if c.gateRecorder == nil {
		return
	}
	// Best effort: the timeline annotates a deploy, it never decides one.
	_ = c.gateRecorder.RecordGateEvent(ctx, store.DeployGateEvent{
		ServiceName: c.serviceName, Image: desired.Image, Phase: phase, Status: status,
		Reason: reason, Message: message, Container: container, At: time.Now(),
	})
}

func (c *Controller) markGateFailure(ctx context.Context, desired *store.DesiredService, reason string) {
	if c.gateRecorder == nil || reason == "" {
		return
	}
	_ = c.gateRecorder.MarkRolloutFailureReason(ctx, c.serviceName, desired.Image, reason)
}

// stabilityGate runs after the first readiness pass. A failed container is
// stopped, so the next pass restarts it under the crash backoff.
func (c *Controller) stabilityGate(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService) error {
	cfg := c.safetyFor(ctx, desired)
	if cfg.SuccessThreshold <= 1 && cfg.StableWindow <= 0 {
		return nil
	}
	inspector, _ := c.runtime.(docker.ExitStateInspector)
	readiness := c.deployReadinessProbeFor(desired)
	var (
		target   probe.Target
		probeCfg probe.Config
		passes   = cfg.SuccessThreshold
	)
	if readiness != nil {
		t, err := c.probeTarget(ctx, state, *readiness, desired.Port, desired.NodeID)
		if err != nil {
			return &gateError{reason: deploysafety.ReasonExitedDuringWindow, message: fmt.Sprintf("stability gate: %v", err)}
		}
		target, probeCfg, passes = t, readiness.ProbeConfig(), 1
	}

	start := time.Now()
	baseline := docker.ExitState{Running: true}
	if inspector != nil {
		if es, err := inspector.InspectExitState(ctx, state.Name); err == nil && es != nil {
			baseline = *es
		}
	}
	startedAt := baseline.StartedAt
	if startedAt.IsZero() {
		startedAt = start
	}
	deadline := start.Add(c.effectiveReadyBudget(desired) + cfg.StableWindow)
	tick := c.gateTick
	if tick <= 0 {
		tick = defaultGateTick
	}
	c.recordGate(ctx, desired, state.Name, deploysafety.PhaseStability, deploysafety.StatusStarted, "",
		fmt.Sprintf("waiting for %d consecutive readiness passes and %s running without a restart", cfg.SuccessThreshold, cfg.StableWindow))

	var lastExit docker.ExitState
	var lastInspectErr error
	for {
		obs := deploysafety.GateObservation{
			Running: true, BaselineRestartCount: baseline.RestartCount, BaselineStartedAt: baseline.StartedAt,
			RestartCount: baseline.RestartCount, StartedAt: baseline.StartedAt,
		}
		unverified := false
		if inspector != nil {
			es, err := inspector.InspectExitState(ctx, state.Name)
			switch {
			case err != nil:
				unverified = true
				lastInspectErr = err
			case es == nil:
				obs.Running = false
			default:
				obs.Running, obs.OOMKilled, obs.ExitCode = es.Running, es.OOMKilled, es.ExitCode
				obs.RestartCount, obs.StartedAt = es.RestartCount, es.StartedAt
				lastExit = *es
			}
		} else if cs, err := c.runtime.InspectByName(ctx, state.Name); err == nil {
			obs.Running = cs != nil && cs.Running
		} else {
			unverified = true
			lastInspectErr = err
		}
		if unverified {
			passes = 0
		} else if readiness == nil {
			passes = cfg.SuccessThreshold
		} else if obs.Running {
			if err := c.prober().Check(ctx, target, probeCfg); err != nil {
				passes = 0
			} else {
				passes++
			}
		}
		now := time.Now()
		obs.ConsecutivePasses = passes
		obs.SinceStart = now.Sub(startedAt)
		obs.BudgetExhausted = !now.Before(deadline)

		verdict, reason := deploysafety.EvaluateGate(obs, cfg)
		switch verdict {
		case deploysafety.VerdictPass:
			c.recordGate(ctx, desired, state.Name, deploysafety.PhaseStability, deploysafety.StatusPassed, "",
				fmt.Sprintf("%d consecutive readiness passes, running %s without a restart", passes, obs.SinceStart.Round(time.Second)))
			return nil
		case deploysafety.VerdictFail:
			msg := gateFailureMessage(state.Name, reason, obs, cfg)
			if unverified && lastInspectErr != nil && reason == deploysafety.ReasonGateTimeout {
				msg = fmt.Sprintf("container %s could not be inspected, so its stability is unverified: %v", state.Name, lastInspectErr)
			}
			if !obs.Running {
				msg = c.captureGateExit(ctx, state, desired, readiness == nil, reason, lastExit, obs)
			}
			c.recordGate(ctx, desired, state.Name, deploysafety.PhaseStability, deploysafety.StatusFailed, reason, msg)
			if obs.Running {
				_ = c.stopContainer(ctx, state.ID)
			}
			return &gateError{reason: reason, message: msg}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("stability gate for %s: %w", state.Name, ctx.Err())
		case <-time.After(tick):
		}
	}
}

// noteImplicitReadiness puts a timeline note on a release that has no
// explicit readiness probe, so the weaker check is visible, not silent. A
// platform probe gets a note too: it is the check that replaced the image's own.
func (c *Controller) noteImplicitReadiness(ctx context.Context, container string, desired *store.DesiredService, readiness *store.ServiceProbe) {
	switch {
	case readiness != nil && readiness.Platform:
		c.recordGate(ctx, desired, container, deploysafety.PhaseReadiness, deploysafety.StatusInfo, "",
			fmt.Sprintf("readiness is a %s probe on port %d run from outside the container", readiness.Kind(), desired.Port))
	case readiness == nil:
		c.recordGate(ctx, desired, container, deploysafety.PhaseReadiness, deploysafety.StatusInfo, "",
			"no port or probe: readiness is only container running; add a port or a health check")
	case readiness.TCP:
		c.recordGate(ctx, desired, container, deploysafety.PhaseReadiness, deploysafety.StatusInfo, "",
			fmt.Sprintf("no readiness probe configured: waiting for a TCP connection on port %d; add a health check for a stronger gate", desired.Port))
	}
}

func gateFailureMessage(name, reason string, obs deploysafety.GateObservation, cfg deploysafety.Effective) string {
	switch reason {
	case deploysafety.ReasonOOMKilledDuringWindow:
		return fmt.Sprintf("container %s was OOM-killed %s after starting, inside its %s stable window", name, obs.SinceStart.Round(time.Second), cfg.StableWindow)
	case deploysafety.ReasonExitedDuringWindow:
		return fmt.Sprintf("container %s exited (code %d) %s after starting, inside its %s stable window", name, obs.ExitCode, obs.SinceStart.Round(time.Second), cfg.StableWindow)
	case deploysafety.ReasonRestartedDuringWindow:
		return fmt.Sprintf("container %s restarted inside its %s stable window", name, cfg.StableWindow)
	default:
		return fmt.Sprintf("container %s did not reach %d consecutive readiness passes within its budget (last streak %d)", name, cfg.SuccessThreshold, obs.ConsecutivePasses)
	}
}

// stabilityGateForNew runs the stability gate for a freshly created
// container, and for any restart of one with no readiness probe: being
// running is its only readiness, so an instant exit would otherwise pass.
func (c *Controller) stabilityGateForNew(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService, created bool) error {
	// A one-shot is meant to exit inside the window; awaitCompletion already judged it.
	if desired.RunsToCompletion() {
		return nil
	}
	if !created && c.deployReadinessProbeFor(desired) != nil {
		return nil
	}
	if !created {
		return c.restartSettle(ctx, state.Name)
	}
	return c.stabilityGate(ctx, state, desired)
}

// restartSettle is a crash restart's whole gate: one live inspect after a
// gate tick. A full stable window here would stall the reconcile loop.
func (c *Controller) restartSettle(ctx context.Context, name string) error {
	tick := c.gateTick
	if tick <= 0 {
		tick = defaultGateTick
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("restart settle for %s: %w", name, ctx.Err())
	case <-time.After(tick):
	}
	if crash := c.exitedCrash(ctx, name); crash != nil {
		return crash
	}
	return nil
}
