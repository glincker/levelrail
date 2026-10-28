package application

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Condition reasons for a service that runs to completion (compose restart: "no" or "on-failure").
const (
	ReasonCompleted         = "Completed"
	ReasonRunningToComplete = "RunningToCompletion"
	ReasonCompletionTimeout = "CompletionTimeout"
)

// EnvOneShotTimeout is how long a run-to-completion service may run before
// it reports CompletionTimeout. A service's own ready_timeout wins over it.
const EnvOneShotTimeout = "APP_DEPLOY_ONESHOT_TIMEOUT"

const defaultOneShotTimeout = 30 * time.Minute

// WithCompletionBudget overrides EnvOneShotTimeout for this controller.
func WithCompletionBudget(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.completionBudget = d }
}

func (c *Controller) effectiveCompletionBudget(desired *store.DesiredService) time.Duration {
	if desired.Health != nil && desired.Health.ReadyTimeout > 0 {
		return desired.Health.ReadyTimeout
	}
	if c.completionBudget > 0 {
		return c.completionBudget
	}
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(EnvOneShotTimeout))); err == nil && d > 0 {
		return d
	}
	return defaultOneShotTimeout
}

// completionPending is a one-shot still running when a pass stops waiting
// for it. It is not a failed deploy, so its container is never stopped.
type completionPending struct{ reason, message string }

func (e *completionPending) Error() string { return e.message }

// stillRunning reports a running one-shot as RunningToCompletion, or as
// CompletionTimeout once it ran past its completion budget.
func (c *Controller) stillRunning(ctx context.Context, name string, desired *store.DesiredService) (string, error) {
	budget := c.effectiveCompletionBudget(desired)
	started := c.oneShotStartedAt(ctx, name)
	if !started.IsZero() && time.Since(started) >= budget {
		return ReasonCompletionTimeout, &completionPending{reason: ReasonCompletionTimeout,
			message: fmt.Sprintf("%s did not run to completion within %s; it is left running", name, budget)}
	}
	return ReasonRunningToComplete, &completionPending{reason: ReasonRunningToComplete, message: fmt.Sprintf("%q is still running to completion", name)}
}

// oneShotStartedAt falls back to the creation time on a transport that does
// not report start times, so a remote one-shot can still time out.
func (c *Controller) oneShotStartedAt(ctx context.Context, name string) time.Time {
	if inspector, ok := c.runtime.(docker.ExitStateInspector); ok {
		if es, err := inspector.InspectExitState(ctx, name); err == nil && es != nil && !es.StartedAt.IsZero() {
			return es.StartedAt
		}
	}
	if cs, err := c.runtime.InspectByName(ctx, name); err == nil && cs != nil {
		return cs.Created
	}
	return time.Time{}
}

// completedExit returns name's exit state when it exited 0 on its own, nil
// otherwise (still running, crashed, OOM killed, or the runtime cannot tell).
func (c *Controller) completedExit(ctx context.Context, name string) *docker.ExitState {
	inspector, ok := c.runtime.(docker.ExitStateInspector)
	if !ok {
		return nil
	}
	es, err := inspector.InspectExitState(ctx, name)
	if err != nil || !CompletedExit(inspector, es) {
		return nil
	}
	return es
}

// CompletedExit reports whether es is a container that ran and exited 0. A
// created, never started one reads the same except for its zero start and
// finish times, which only a transport that reports times can tell apart.
func CompletedExit(src docker.ExitStateInspector, es *docker.ExitState) bool {
	if es == nil || es.Running || es.OOMKilled || es.ExitCode != 0 {
		return false
	}
	if es.StartedAt.IsZero() || es.FinishedAt.IsZero() {
		return !reportsExitTimes(src)
	}
	return true
}

// exitTimesReporter lets a runtime declare that it fills StartedAt and FinishedAt.
type exitTimesReporter interface{ ReportsExitTimes() bool }

func reportsExitTimes(src docker.ExitStateInspector) bool {
	if r, ok := src.(exitTimesReporter); ok {
		return r.ReportsExitTimes()
	}
	_, local := src.(*docker.Client)
	return local
}

// settleOneShot handles an existing container of a run-to-completion service.
// done is false only for a nonzero exit, which keeps today's crash backoff restart.
func (c *Controller) settleOneShot(ctx context.Context, target string, state *docker.ContainerState, desired *store.DesiredService) (out replicaOutcome, done bool, err error) {
	if c.completedExit(ctx, target) != nil {
		c.restartBackoff.observeRunning(target, time.Now())
		out, err = c.confirmedOutcome(ctx, target, state, desired, false)
		return out, true, err
	}
	if !state.Running {
		return replicaOutcome{}, false, nil
	}
	if crash := c.exitedCrash(ctx, target); crash != nil {
		return replicaOutcome{}, false, nil
	}
	reason, err := c.stillRunning(ctx, target, desired)
	return replicaOutcome{reason: reason}, true, err
}

// awaitCompletion waits up to the ready budget for a freshly started one-shot
// to exit: 0 passes and any other exit fails like a crash. Still running is a
// *completionPending, judged against the completion budget on later passes.
func (c *Controller) awaitCompletion(ctx context.Context, name string, desired *store.DesiredService) error {
	if _, ok := c.runtime.(docker.ExitStateInspector); !ok {
		return nil
	}
	deadline := time.Now().Add(min(c.effectiveReadyBudget(desired), c.effectiveCompletionBudget(desired)))
	for {
		if c.completedExit(ctx, name) != nil {
			return nil
		}
		if crash := c.exitedCrash(ctx, name); crash != nil {
			return crash
		}
		left := time.Until(deadline)
		if left <= 0 {
			_, err := c.stillRunning(ctx, name, desired)
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s to complete: %w", name, ctx.Err())
		case <-time.After(min(startupPollInterval, left)):
		}
		if c.desiredSuperseded(ctx, desired) {
			return errSuperseded
		}
	}
}

// oneShotsDone reports whether every target of a run-to-completion service exited 0.
func (c *Controller) oneShotsDone(ctx context.Context, targets []string, desired *store.DesiredService) bool {
	if !desired.RunsToCompletion() {
		return false
	}
	for _, t := range targets {
		if c.completedExit(ctx, t) == nil {
			return false
		}
	}
	return true
}
