package scheduledtask

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/store"
)

// errReplacedByNewRun is the context.Cause a run's context carries once
// concurrency_policy "replace" has cancelled it in favor of a newer
// invocation of the same task, distinguishing that outcome from a real
// timeout or a caller-driven shutdown at every point runCtx's
// cancellation is observed.
var errReplacedByNewRun = errors.New("scheduledtask: run replaced by a newer invocation")

// isReplaced reports whether ctx (a runCtx returned by beginRun) was
// cancelled specifically by a concurrency_policy "replace" takeover,
// as opposed to any other cancellation (a genuine timeout, the outer
// context shutting down).
func isReplaced(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errReplacedByNewRun)
}

// inFlightRun tracks one currently-executing Run call for a given task
// ID: cancel lets a later "replace" call pre-empt it, done is closed once
// its own cleanup has run, so a replace can wait for it to actually stop
// before starting the new invocation.
type inFlightRun struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
}

// beginRun registers this invocation as the in-flight run for taskID and
// applies policy against whatever run (if any) is already registered:
//
//   - allow: always proceeds, registering itself alongside whatever else
//     is in flight.
//   - forbid: if a previous run is still in flight, skip is true and the
//     caller must record ScheduledTaskStatusSkippedConcurrency instead of
//     running anything.
//   - replace: cancels the in-flight run's context (its own select loop
//     observes this via isReplaced and records ScheduledTaskStatusReplaced)
//     and waits for it to finish cleaning up before this run proceeds, so
//     the two runs' RecordScheduledTaskRun calls can never land out of
//     order.
//
// The returned cleanup must run (via defer) once the caller's own Run
// call is done, whatever the outcome; it never panics or blocks.
func (r *Runner) beginRun(ctx context.Context, taskID, policy string) (runCtx context.Context, cleanup func(), skip bool) {
	r.mu.Lock()
	if r.inFlight == nil {
		r.inFlight = make(map[string]*inFlightRun)
	}
	prev, exists := r.inFlight[taskID]

	if exists && policy == store.ScheduledTaskConcurrencyForbid {
		r.mu.Unlock()
		return nil, nil, true
	}
	if exists && policy == store.ScheduledTaskConcurrencyReplace {
		prev.cancel(errReplacedByNewRun)
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	entry := &inFlightRun{cancel: cancel, done: done}
	r.inFlight[taskID] = entry
	r.mu.Unlock()

	if exists && policy == store.ScheduledTaskConcurrencyReplace {
		<-prev.done
	}

	cleanup = func() {
		close(done)
		r.mu.Lock()
		if r.inFlight[taskID] == entry {
			delete(r.inFlight, taskID)
		}
		r.mu.Unlock()
	}
	return runCtx, cleanup, false
}

// normalizeConcurrencyPolicy defaults an empty policy to allow, the same
// "empty means today's existing behavior" convention
// store.ScheduledTask.ConcurrencyPolicy's own doc comment establishes:
// a task saved before this field existed reads back as allow via the
// store's own normalization, but a caller constructing a ScheduledTask by
// hand (a test, a fixture) may still leave it empty.
func normalizeConcurrencyPolicy(policy string) string {
	if policy == "" {
		return store.ScheduledTaskConcurrencyAllow
	}
	return policy
}
