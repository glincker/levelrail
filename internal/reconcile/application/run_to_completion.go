package application

import (
	"context"
	"fmt"
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

// completedExit returns name's exit state when it exited 0 on its own, nil
// otherwise (still running, crashed, OOM killed, or the runtime cannot tell).
func (c *Controller) completedExit(ctx context.Context, name string) *docker.ExitState {
	inspector, ok := c.runtime.(docker.ExitStateInspector)
	if !ok {
		return nil
	}
	es, err := inspector.InspectExitState(ctx, name)
	if err != nil || es == nil || es.Running || es.OOMKilled || es.ExitCode != 0 {
		return nil
	}
	return es
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
	return replicaOutcome{reason: ReasonRunningToComplete}, true, fmt.Errorf("%q is still running to completion", target)
}

// awaitCompletion waits for a freshly started one-shot to exit: 0 passes, any
// other exit fails like a crash, and still running past the ready budget fails.
func (c *Controller) awaitCompletion(ctx context.Context, name string, desired *store.DesiredService) error {
	if _, ok := c.runtime.(docker.ExitStateInspector); !ok {
		return nil
	}
	budget := c.effectiveReadyBudget(desired)
	deadline := time.Now().Add(budget)
	for {
		if c.completedExit(ctx, name) != nil {
			return nil
		}
		if crash := c.exitedCrash(ctx, name); crash != nil {
			return crash
		}
		left := time.Until(deadline)
		if left <= 0 {
			return &gateError{reason: ReasonCompletionTimeout, message: fmt.Sprintf("%s did not run to completion within %s", name, budget)}
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
