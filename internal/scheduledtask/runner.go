// Package scheduledtask execs an operator-defined command into a
// running app's container on a cron schedule (e.g. a nightly cleanup
// script, a periodic cache-warm job), mirroring internal/backup's own
// Runner/Scheduler split: Runner (this file) does one run and records
// its outcome, Scheduler (scheduler.go) decides when a run is due and
// calls Runner. The same Runner instance backs both the background
// cron loop and the "run now" HTTP trigger (internal/api), so there is
// exactly one place that actually execs a command and records what
// happened.
package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultRunTimeout bounds how long Runner.Run waits for a scheduled
// command to finish, the same "hard ceiling, not a suggestion" reasoning
// internal/api/exec.go's own defaultExecTimeout doc comment gives,
// sized longer here (a background tick, not a held-open HTTP
// connection) so a nightly cleanup script or a slow cache warm has real
// room to finish.
const defaultRunTimeout = 10 * time.Minute

// maxOutputBytes caps how much of a command's output Runner keeps: the
// tail (most recent bytes), not the head, since the last lines of a
// failing script are usually the actual error and matter more than its
// first lines. This is stored back onto the scheduled_tasks row itself
// (store.ScheduledTask.LastRunOutput), never streamed, so it has to fit
// comfortably in one column, the same "don't store unbounded output"
// concern internal/api/exec.go's own execMaxOutputBytes already applies
// to the one-off HTTP exec path.
const maxOutputBytes = 64 * 1024

// AppStore is the narrow store surface Runner needs to resolve a
// scheduled task's target service, the same "consumer defines its own
// narrow interface" convention internal/backup.ScheduleStore already
// establishes for this package's sibling.
type AppStore interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// RunStore is the store surface Runner needs to persist a run's outcome
// back onto the task's own row (RecordScheduledTaskRun,
// internal/store/scheduled_task.go).
type RunStore interface {
	RecordScheduledTaskRun(ctx context.Context, id string, ranAt time.Time, status, output string) error
}

// NodeRuntimeResolver resolves the docker.Runtime for a given node ID,
// identical in shape to internal/api's own NodeRuntimeResolver
// (router.go): both close over the same resolveNodeTransport helper in
// cmd/levelrail/main.go, redeclared here rather than imported across
// the package boundary, the same reasoning
// backup.ScheduledBackupRunner's own doc comment gives for its own
// redeclared interfaces.
type NodeRuntimeResolver func(nodeID string) (docker.Runtime, error)

// Runner execs one scheduled task's command into its target service's
// currently running container and records the outcome. Used both by
// Scheduler (a due cron tick) and by the "run now" HTTP handler
// (internal/api): both need the identical exec-and-record behavior, not
// two independent copies of it.
type Runner struct {
	Apps     AppStore
	Runs     RunStore
	Resolver NodeRuntimeResolver
	Logger   *slog.Logger

	// Now returns the current time, the same testable-clock field
	// backup.Runner.Now and backup.Scheduler.Now already establish:
	// production code leaves it nil (falls back to time.Now), tests set
	// it for deterministic timestamps.
	Now func() time.Time

	// Timeout overrides defaultRunTimeout when positive; zero (the
	// production default) falls back to it. Exists purely so a test can
	// exercise the timeout branch in milliseconds instead of actually
	// waiting defaultRunTimeout out.
	Timeout time.Duration
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return defaultRunTimeout
}

func (r *Runner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// Run execs task.Command inside task.ServiceName's currently running
// container and records the outcome (RecordScheduledTaskRun): success,
// a nonzero exit ("failed"), a timeout, or the container simply not
// being up right now ("container_not_running"). The returned error is
// only ever a genuine infrastructure failure this call itself hit
// (loading the app, or recording the result once a command has already
// run): a command that ran but failed, or a container that wasn't
// running, are both normal, expected outcomes recorded on the task's
// own row, never returned as an error, the same "a nonzero exit is a
// 200, not a failure" distinction execResponse's own doc comment draws
// for the one-off HTTP exec path.
func (r *Runner) Run(ctx context.Context, task store.ScheduledTask) error {
	svc, err := r.Apps.GetDesiredService(ctx, task.ServiceName)
	if err != nil {
		return fmt.Errorf("scheduledtask: run %q: load service %q: %w", task.ID, task.ServiceName, err)
	}

	rt, err := r.Resolver(svc.NodeID)
	if err != nil {
		r.log().Warn("scheduledtask: resolve node runtime failed",
			slog.String("task_id", task.ID), slog.String("service", task.ServiceName), slog.String("error", err.Error()))
		return r.record(ctx, task.ID, store.ScheduledTaskStatusContainerNotRunning, "node unreachable: "+err.Error())
	}

	target := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)
	state, err := rt.InspectByName(ctx, target)
	if err != nil {
		return fmt.Errorf("scheduledtask: run %q: inspect container %q: %w", task.ID, target, err)
	}
	if state == nil || !state.Running {
		return r.record(ctx, task.ID, store.ScheduledTaskStatusContainerNotRunning, "container not running")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	rc, err := rt.Exec(execCtx, state.ID, task.Command)
	if err != nil {
		return r.record(ctx, task.ID, store.ScheduledTaskStatusFailed, "exec failed: "+err.Error())
	}

	type outcome struct {
		out *tailCappedWriter
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		capped := newTailCappedWriter(maxOutputBytes)
		_, readErr := io.Copy(capped, rc)
		done <- outcome{out: capped, err: readErr}
	}()

	select {
	case res := <-done:
		_ = rc.Close()
		return r.recordExecOutcome(ctx, task.ID, res.out, res.err)
	case <-execCtx.Done():
		// Same best-effort-not-guaranteed cleanup handleExecApp's own
		// timeout branch documents: closing rc unblocks a command still
		// producing output, but does not guarantee a genuinely hung
		// command (blocked on its own container-side read) stops
		// immediately. What this guarantees is narrower and still the
		// thing that matters here: this goroutine never blocks past
		// defaultRunTimeout.
		_ = rc.Close()
		return r.record(ctx, task.ID, store.ScheduledTaskStatusTimeout, fmt.Sprintf("command timed out after %s", r.timeout()))
	}
}

func (r *Runner) recordExecOutcome(ctx context.Context, taskID string, out *tailCappedWriter, err error) error {
	if err == nil {
		return r.record(ctx, taskID, store.ScheduledTaskStatusSuccess, out.String())
	}

	var execErr *docker.ExecExitError
	if errors.As(err, &execErr) {
		output := out.String()
		if execErr.Stderr != "" {
			output += "\n--- stderr ---\n" + execErr.Stderr
		}
		return r.record(ctx, taskID, store.ScheduledTaskStatusFailed, output)
	}

	return r.record(ctx, taskID, store.ScheduledTaskStatusFailed, "reading command output failed: "+err.Error())
}

func (r *Runner) record(ctx context.Context, taskID, status, output string) error {
	if err := r.Runs.RecordScheduledTaskRun(ctx, taskID, r.now(), status, capTail(output, maxOutputBytes)); err != nil {
		return fmt.Errorf("scheduledtask: record run %q: %w", taskID, err)
	}
	return nil
}

// capTail trims s down to its last limit bytes, the same "keep the
// tail, not the head" reasoning maxOutputBytes's own doc comment gives:
// a defensive bound on top of tailCappedWriter's own cap, since a
// non-exec outcome (e.g. "node unreachable: <err>") is built from a Go
// error string this package doesn't otherwise bound.
func capTail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}

// tailCappedWriter keeps only the most recent limit bytes written to
// it, the tail counterpart to internal/api/exec.go's own cappedWriter
// (which keeps the head/first bytes instead): see maxOutputBytes's own
// doc comment for why a scheduled task keeps the end of a command's
// output. Every Write reports full success regardless of the cap, the
// same contract cappedWriter's own doc comment establishes and for the
// identical reason: this is fed by io.Copy draining Exec's stream to
// completion, which must not stop early just because the cap was hit,
// or the command's trailing error (its real exit code) would never be
// reached.
type tailCappedWriter struct {
	limit int
	buf   []byte
	total int64
}

func newTailCappedWriter(limit int) *tailCappedWriter {
	return &tailCappedWriter{limit: limit, buf: make([]byte, 0, limit)}
}

func (w *tailCappedWriter) Write(p []byte) (int, error) {
	w.total += int64(len(p))
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.limit {
		trimmed := make([]byte, w.limit)
		copy(trimmed, w.buf[len(w.buf)-w.limit:])
		w.buf = trimmed
	}
	return len(p), nil
}

func (w *tailCappedWriter) truncated() bool {
	return w.total > int64(w.limit)
}

func (w *tailCappedWriter) String() string {
	if w.truncated() {
		return "[output truncated, showing the last " + strconv.Itoa(w.limit) + " bytes]\n" + string(w.buf)
	}
	return string(w.buf)
}
