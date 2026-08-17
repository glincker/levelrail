// Package scheduledtask runs an app-level scheduled task's command
// inside its service's currently running container, on demand (Runner,
// the manual "run now" trigger internal/api dispatches through) or on a
// cron tick (Scheduler, scheduler.go). Both share this file's Runner:
// the same "one implementation, two invocation paths" shape
// internal/backup.Runner already establishes for backups (see that
// package's own runner.go and scheduler.go).
//
// Command execution itself reuses docker.Runtime.Exec, the identical
// Docker Engine API primitive POST /apps/{name}/exec
// (internal/api/exec.go) already drives: this package duplicates that
// handler's small container-resolution and output-capping logic rather
// than importing internal/api, the same package-boundary direction
// internal/backup already keeps (internal/api depends on internal/backup,
// never the reverse).
package scheduledtask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultTimeout bounds how long Run waits for a task's command to
// finish when the task itself specifies no TimeoutSeconds override,
// longer than exec.go's own defaultExecTimeout (30s): a scheduled task is
// typically a batch job (Laravel's schedule:run, a cleanup script), not
// an interactive one-liner, so it gets more room by default.
const DefaultTimeout = 5 * time.Minute

// maxOutputBytes caps how much of a run's combined stdout/stderr this
// package holds in memory and persists to scheduled_task_runs.output,
// the same "an unbounded buffer fed by a command this package doesn't
// control is not acceptable" reasoning exec.go's own execMaxOutputBytes
// applies.
const maxOutputBytes = 1 << 20 // 1 MiB

// NodeRuntimeResolver picks the docker.Runtime that owns a given node,
// redeclared here rather than reusing api.NodeRuntimeResolver: this
// package must not import internal/api (see this file's own package doc
// comment). cmd/levelrail/main.go wires the identical
// resolveNodeTransport closure into both.
type NodeRuntimeResolver func(nodeID string) (docker.Runtime, error)

// AppStore is the narrow store surface Runner needs to resolve a task's
// owning service.
type AppStore interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// HistoryStore is the narrow store surface Runner needs to record a run.
type HistoryStore interface {
	StartScheduledTaskRun(ctx context.Context, r store.ScheduledTaskRun) error
	FinishScheduledTaskRun(ctx context.Context, id, status string, exitCode int, output, errMsg, finishedAt string) error
}

// Runner executes one ScheduledTask's command and records the attempt in
// store.ScheduledTaskRun throughout, the same "running row first, then
// succeeded/failed" shape backup.Runner.RunBackup already establishes.
type Runner struct {
	Apps    AppStore
	Runtime NodeRuntimeResolver
	History HistoryStore
	// Now returns the current time; nil in production (Run falls back to
	// time.Now), overridden in tests for deterministic timestamps.
	Now func() time.Time
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Run executes task once, recording the attempt under runID (minted by
// the caller, the same "known before the first write" convention
// store.BackupHistory's own ID already follows). The returned error is
// the command's own real failure (a nonzero exit, a container that
// wasn't running, a transport failure), already persisted into the run's
// own Error/Status fields before this returns: the one exception is a
// failure to write FinishScheduledTaskRun itself, which this can't
// recover from and returns as-is, mirroring RunBackup's identical
// carve-out.
func (r *Runner) Run(ctx context.Context, runID string, task store.ScheduledTask) error {
	startedAt := r.now()
	if err := r.History.StartScheduledTaskRun(ctx, store.ScheduledTaskRun{
		ID:              runID,
		ScheduledTaskID: task.ID,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("scheduledtask: start run %q: %w", runID, err)
	}

	exitCode, output, truncated, runErr := r.execTask(ctx, task)
	if truncated {
		output += "\n... (output truncated)"
	}

	status := store.ScheduledTaskRunStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.ScheduledTaskRunStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.History.FinishScheduledTaskRun(ctx, runID, status, exitCode, output, errMsg, finishedAt); err != nil {
		return fmt.Errorf("scheduledtask: finish run %q: %w", runID, err)
	}
	return runErr
}

// execTask resolves task's owning service, execs its Command as
// `sh -c Command` in that service's running container, and drains the
// result, mirroring handleExecApp's own resolve-inspect-exec sequence
// (internal/api/exec.go) end to end.
func (r *Runner) execTask(ctx context.Context, task store.ScheduledTask) (exitCode int, output string, truncated bool, err error) {
	svc, err := r.Apps.GetDesiredService(ctx, task.ServiceName)
	if err != nil {
		return 0, "", false, fmt.Errorf("load app %q: %w", task.ServiceName, err)
	}

	rt, err := r.Runtime(svc.NodeID)
	if err != nil {
		return 0, "", false, fmt.Errorf("resolve node runtime: %w", err)
	}

	target := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)
	state, err := rt.InspectByName(ctx, target)
	if err != nil {
		return 0, "", false, fmt.Errorf("inspect container %q: %w", target, err)
	}
	if state == nil || !state.Running {
		return 0, "", false, fmt.Errorf("app %q has no running container", task.ServiceName)
	}

	timeout := DefaultTimeout
	if task.TimeoutSeconds > 0 {
		timeout = time.Duration(task.TimeoutSeconds) * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rc, err := rt.Exec(execCtx, state.ID, []string{"sh", "-c", task.Command})
	if err != nil {
		return 0, "", false, fmt.Errorf("start exec: %w", err)
	}

	type outcome struct {
		out *cappedWriter
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		capped := &cappedWriter{limit: maxOutputBytes}
		_, readErr := io.Copy(capped, rc)
		done <- outcome{out: capped, err: readErr}
	}()

	select {
	case res := <-done:
		_ = rc.Close()
		return exitAndOutput(res.out, res.err)
	case <-execCtx.Done():
		// Best-effort unblock, not a guarantee: see exec.go's own
		// identical timeout branch doc comment for why a genuinely hung
		// command's own goroutine may still be running after this
		// returns, and why that's an acceptable, pre-existing gap rather
		// than something this call site needs to solve.
		_ = rc.Close()
		return 0, "", false, fmt.Errorf("command timed out after %s", timeout)
	}
}

// exitAndOutput turns Exec's drained stream into (exitCode, output,
// truncated, err), the scheduled-task counterpart of exec.go's own
// writeExecOutcome: a nil err or a *docker.ExecExitError both mean the
// exec mechanism itself worked (a nonzero exit is not itself a
// mechanism failure), anything else is a genuine transport failure.
func exitAndOutput(out *cappedWriter, err error) (exitCode int, output string, truncated bool, retErr error) {
	stdout := out.buf.String()
	truncated = out.truncated()

	if err == nil {
		return 0, stdout, truncated, nil
	}

	var execErr *docker.ExecExitError
	if errors.As(err, &execErr) {
		combined := stdout
		if execErr.Stderr != "" {
			if combined != "" {
				combined += "\n"
			}
			combined += "stderr: " + execErr.Stderr
		}
		return execErr.ExitCode, combined, truncated, fmt.Errorf("command exited %d", execErr.ExitCode)
	}

	return 0, stdout, truncated, fmt.Errorf("read exec output: %w", err)
}

// cappedWriter accumulates up to limit bytes and silently discards the
// rest while still reporting every write as successful, so io.Copy keeps
// draining a command's stdout to completion past the cap instead of
// stopping early and never reaching Exec's trailing exit-code error. See
// exec.go's own identical cappedWriter for the full reasoning.
type cappedWriter struct {
	buf   bytes.Buffer
	limit int
	total int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	c.total += int64(n)
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if n > remaining {
			p = p[:remaining]
		}
		c.buf.Write(p)
	}
	return n, nil
}

func (c *cappedWriter) truncated() bool {
	return c.total > int64(c.limit)
}
