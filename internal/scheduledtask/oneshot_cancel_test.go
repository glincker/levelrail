package scheduledtask

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// killedRuntime blocks until the run is cancelled, then reports what a
// killed container leaves: partial output and a kill exit code.
type killedRuntime struct {
	*fakeRuntime
	started chan struct{}
}

func (k *killedRuntime) RunToCompletion(ctx context.Context, _ docker.ContainerSpec, _ int) (docker.RunOutcome, error) {
	close(k.started)
	<-ctx.Done()
	return docker.RunOutcome{ContainerID: "c1", Output: "partial\n", OutputCaptured: true, ExitCode: 137}, ctx.Err()
}

func TestRunAttempt_OperatorCancelRecordsResult(t *testing.T) {
	rt := &killedRuntime{fakeRuntime: &fakeRuntime{}, started: make(chan struct{})}
	env := newJobEnv(t, rt, fakeSpecs{}, func(task *store.ScheduledTask) { task.Mode = store.ScheduledTaskModeFresh })

	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		<-rt.started
		cancel(ErrRunCancelled)
	}()
	done := make(chan error, 1)
	go func() {
		_, err := env.runner.RunAttempt(ctx, env.task)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAttempt: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunAttempt did not return after cancel")
	}

	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusInterrupted || run.Message != cancelledMessage {
		t.Fatalf("status = %q, message = %q, want interrupted with the cancel message", run.Status, run.Message)
	}
	if run.ExitCode == nil || *run.ExitCode != 137 {
		t.Fatalf("exit code = %v, want 137", run.ExitCode)
	}
	if run.Output != "partial\n" {
		t.Fatalf("output = %q, want the partial output", run.Output)
	}
}
