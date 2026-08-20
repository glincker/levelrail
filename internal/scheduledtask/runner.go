// Package scheduledtask executes an app-level scheduled task's command in
// its service's container, shared by the on-demand Runner and the
// cron-triggered Scheduler (scheduler.go). It duplicates exec.go's
// container-resolution logic instead of importing internal/api, keeping
// that dependency one-directional.
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

// DefaultTimeout bounds Run when a task specifies no TimeoutSeconds,
// longer than exec.go's 30s default since scheduled tasks are typically
// batch jobs, not interactive one-liners.
const DefaultTimeout = 5 * time.Minute

// maxOutputBytes caps how much of a run's output this package buffers
// and persists, mirroring exec.go's execMaxOutputBytes.
const maxOutputBytes = 1 << 20 // 1 MiB

// NodeRuntimeResolver picks the docker.Runtime that owns a given node.
// Redeclared here instead of reusing api.NodeRuntimeResolver since this
// package must not import internal/api.
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

// Runner executes one ScheduledTask's command, recording a running row
// first and a succeeded/failed row after, mirroring backup.Runner.
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

// Run executes task once under runID, recording the attempt before
// returning. The returned error is the command's own failure already
// persisted, except a failure to write FinishScheduledTaskRun itself.
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

// execTask resolves task's owning service, execs Command as `sh -c
// Command` in its running container, and drains the result.
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
		// Best-effort unblock only: the read goroutine may still be
		// running after this returns (see exec.go's identical branch).
		_ = rc.Close()
		return 0, "", false, fmt.Errorf("command timed out after %s", timeout)
	}
}

// exitAndOutput turns Exec's drained stream into (exitCode, output,
// truncated, err). A nil err or *docker.ExecExitError both mean the exec
// mechanism worked; anything else is a genuine transport failure.
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

// cappedWriter accumulates up to limit bytes, discarding the rest while
// reporting writes as successful so io.Copy keeps draining past the cap
// instead of stopping before Exec's trailing exit code. Mirrors
// exec.go's cappedWriter.
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
