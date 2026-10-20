package docker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// RunOutcome is what a run-to-completion container produced.
type RunOutcome struct {
	ContainerID string
	ExitCode    int
	OOMKilled   bool
	// Output is the tail of the container's combined stdout and stderr.
	// OutputCaptured is false when the node's transport cannot return logs.
	Output         string
	OutputCaptured bool
}

// RunToCompletionRuntime is an optional Runtime capability: create, start,
// wait for and remove a container built from a full ContainerSpec, returning
// its exit code and output. The error is context.Canceled or
// context.DeadlineExceeded (wrapped) when ctx ended the run; the container
// is killed and removed on every path.
type RunToCompletionRuntime interface {
	RunToCompletion(ctx context.Context, spec ContainerSpec, maxOutput int) (RunOutcome, error)
}

const jobPollInterval = time.Second

// RunToCompletionVia runs spec on rt, using its native implementation when
// it has one and otherwise create, start and poll through the plain Runtime
// surface (exit code only, no output), which is all a remote agent exposes.
func RunToCompletionVia(ctx context.Context, rt Runtime, spec ContainerSpec, maxOutput int) (RunOutcome, error) {
	if native, ok := rt.(RunToCompletionRuntime); ok {
		return native.RunToCompletion(ctx, spec, maxOutput)
	}
	inspector, ok := rt.(ExitStateInspector)
	if !ok {
		return RunOutcome{}, errors.New("docker: this node cannot report a container's exit state, run the job in exec mode")
	}
	id, err := rt.Create(ctx, spec)
	if err != nil {
		return RunOutcome{}, fmt.Errorf("create job container %q: %w", spec.Name, err)
	}
	out := RunOutcome{ContainerID: id}
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = rt.Remove(cctx, id, true)
	}()
	if err := rt.Start(ctx, id); err != nil {
		return out, fmt.Errorf("start job container %q: %w", spec.Name, err)
	}
	ticker := time.NewTicker(jobPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			_ = rt.Stop(cctx, id, 0)
			cancel()
			return out, fmt.Errorf("job container %q: %w", spec.Name, ctx.Err())
		case <-ticker.C:
			st, err := inspector.InspectExitState(ctx, spec.Name)
			if err != nil {
				return out, fmt.Errorf("inspect job container %q: %w", spec.Name, err)
			}
			if st == nil {
				return out, fmt.Errorf("job container %q disappeared before reporting an exit state", spec.Name)
			}
			if !st.Running {
				out.ExitCode, out.OOMKilled = st.ExitCode, st.OOMKilled
				return out, nil
			}
		}
	}
}

// RunToCompletion implements RunToCompletionRuntime through Create, so the
// job container gets the same hardening, image pull and network handling a
// service container does.
func (c *Client) RunToCompletion(ctx context.Context, spec ContainerSpec, maxOutput int) (RunOutcome, error) {
	id, err := c.Create(ctx, spec)
	if err != nil {
		return RunOutcome{}, fmt.Errorf("create job container %q: %w", spec.Name, err)
	}
	out := RunOutcome{ContainerID: id, OutputCaptured: true}
	defer c.removeQuietly(id)

	waitCh, errCh := c.cli.ContainerWait(ctx, id, container.WaitConditionNextExit)
	if err := c.Start(ctx, id); err != nil {
		return out, fmt.Errorf("start job container %q: %w", spec.Name, err)
	}

	var runErr error
	select {
	case w := <-waitCh:
		out.ExitCode = int(w.StatusCode)
	case err := <-errCh:
		runErr = fmt.Errorf("wait for job container %q: %w", spec.Name, err)
	case <-ctx.Done():
		kctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_ = c.cli.ContainerKill(kctx, id, "SIGKILL")
		cancel()
		runErr = fmt.Errorf("job container %q: %w", spec.Name, ctx.Err())
	}

	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if info, err := c.cli.ContainerInspect(lctx, id); err == nil && info.State != nil {
		out.OOMKilled = info.State.OOMKilled
	}
	logs, err := c.cli.ContainerLogs(lctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		if runErr == nil {
			runErr = fmt.Errorf("read job container %q logs: %w", spec.Name, err)
		}
		return out, runErr
	}
	defer func() { _ = logs.Close() }()
	tail := &tailBuffer{limit: maxOutput}
	_, _ = stdcopy.StdCopy(tail, tail, logs)
	out.Output = tail.String()
	return out, runErr
}

// tailBuffer keeps the most recent limit bytes written to it.
type tailBuffer struct {
	limit     int
	buf       []byte
	truncated bool
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if t.limit > 0 && len(t.buf) > t.limit {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.limit:]...)
		t.truncated = true
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	if t.truncated {
		return "[output truncated, showing the last bytes]\n" + string(t.buf)
	}
	return string(t.buf)
}
