package scheduledtask

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nativeRuntime is a fakeRuntime that also implements
// docker.RunToCompletionRuntime, the local-node shape.
type nativeRuntime struct {
	*fakeRuntime
	mu       sync.Mutex
	outcomes []docker.RunOutcome
	errs     []error
	specs    []docker.ContainerSpec
	hold     chan struct{}
}

func (n *nativeRuntime) RunToCompletion(ctx context.Context, spec docker.ContainerSpec, _ int) (docker.RunOutcome, error) {
	n.mu.Lock()
	idx := len(n.specs)
	n.specs = append(n.specs, spec)
	n.mu.Unlock()
	if n.hold != nil {
		select {
		case <-n.hold:
		case <-ctx.Done():
			return docker.RunOutcome{}, ctx.Err()
		}
	}
	var out docker.RunOutcome
	if idx < len(n.outcomes) {
		out = n.outcomes[idx]
	} else if len(n.outcomes) > 0 {
		out = n.outcomes[len(n.outcomes)-1]
	}
	var err error
	if idx < len(n.errs) {
		err = n.errs[idx]
	}
	return out, err
}

// pollRuntime exercises the generic Create/Start/poll path a remote agent
// transport gets, with injectable half-success failures.
type pollRuntime struct {
	*fakeRuntime
	mu        sync.Mutex
	startErr  error
	running   bool
	exitCode  int
	removed   []string
	stopped   []string
	createdAs string
}

func (p *pollRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createdAs = spec.Name
	return "job-container", nil
}
func (p *pollRuntime) Start(context.Context, string) error { return p.startErr }
func (p *pollRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, id)
	return nil
}
func (p *pollRuntime) Remove(_ context.Context, id string, force bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if force {
		p.removed = append(p.removed, id)
	}
	return nil
}
func (p *pollRuntime) InspectExitState(context.Context, string) (*docker.ExitState, error) {
	return &docker.ExitState{Running: p.running, ExitCode: p.exitCode}, nil
}

type fakeSpecs struct {
	secrets []string
	err     error
}

func (f fakeSpecs) BuildJobSpec(_ context.Context, svc *store.DesiredService, _ docker.Runtime, name string, command []string, res *docker.Resources) (application.JobSpec, error) {
	if f.err != nil {
		return application.JobSpec{}, f.err
	}
	spec := docker.ContainerSpec{Name: name, Image: svc.Image, Entrypoint: command[:1], Command: command[1:], Resources: res}
	return application.JobSpec{Spec: spec, SecretValues: f.secrets}, nil
}

func (f fakeSpecs) JobSecretValues(context.Context, *store.DesiredService, docker.Runtime) ([]string, error) {
	return f.secrets, nil
}

type jobEnv struct {
	db     *store.DB
	runner *Runner
	task   store.ScheduledTask
	sleeps []time.Duration
}

func newJobEnv(t *testing.T, rt docker.Runtime, specs SpecBuilder, mut func(*store.ScheduledTask)) *jobEnv {
	t.Helper()
	db := openDB(t)
	task := saveJob(t, db, "st_1", "0 3 * * *", mut)
	env := &jobEnv{db: db, task: task}
	env.runner = &Runner{
		Apps: db, Runs: db, History: db, Specs: specs, NamePrefix: "lr",
		Resolver: func(string) (docker.Runtime, error) { return rt, nil },
		Now:      func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) },
		Sleep: func(_ context.Context, d time.Duration) error {
			env.sleeps = append(env.sleeps, d)
			return nil
		},
	}
	return env
}

func (e *jobEnv) lastRun(t *testing.T) store.ScheduledTaskRun {
	t.Helper()
	runs, err := e.db.ListScheduledTaskRuns(context.Background(), store.ScheduledTaskRunFilter{ServiceName: "web"})
	if err != nil || len(runs) == 0 {
		t.Fatalf("no history rows: %v", err)
	}
	full, err := e.db.GetScheduledTaskRun(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatalf("GetScheduledTaskRun: %v", err)
	}
	return full
}

func TestRunner_Fresh_SuccessPersistsHistoryAndRedactsSecrets(t *testing.T) {
	rt := &nativeRuntime{fakeRuntime: &fakeRuntime{}, outcomes: []docker.RunOutcome{{ContainerID: "c9", Output: "token=hunter2-secret ok\n", OutputCaptured: true}}}
	env := newJobEnv(t, rt, fakeSpecs{secrets: []string{"hunter2-secret"}}, func(j *store.ScheduledTask) {
		j.MemoryLimitMB, j.CPULimitMillis = 256, 500
	})

	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerAPI}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusSuccess || run.Trigger != store.ScheduledTaskTriggerAPI || run.Attempts != 1 {
		t.Errorf("run = %+v, want success via api in 1 attempt", run)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", run.ExitCode)
	}
	if strings.Contains(run.Output, "hunter2-secret") || !strings.Contains(run.Output, redactedMarker) {
		t.Errorf("Output = %q, want the secret redacted", run.Output)
	}
	task, _ := env.db.GetScheduledTask(context.Background(), "st_1")
	if strings.Contains(task.LastRunOutput, "hunter2-secret") {
		t.Errorf("last_run_output leaked a secret: %q", task.LastRunOutput)
	}
	spec := rt.specs[0]
	if spec.Resources == nil || spec.Resources.MemoryBytes != 256<<20 || spec.Resources.NanoCPUs != 500_000_000 {
		t.Errorf("resources = %+v, want 256MiB and 0.5 cpu", spec.Resources)
	}
	if !strings.HasPrefix(spec.Name, "lr-job-web-") {
		t.Errorf("container name = %q, want the brand prefixed job name", spec.Name)
	}
}

func TestRunner_Retries_BackoffAndOutcome(t *testing.T) {
	fail := docker.RunOutcome{ExitCode: 3, Output: "boom", OutputCaptured: true}
	ok := docker.RunOutcome{ExitCode: 0, Output: "fine", OutputCaptured: true}
	tests := []struct {
		name         string
		outcomes     []docker.RunOutcome
		retries      int
		wantStatus   string
		wantAttempts int
		wantSleeps   []time.Duration
	}{
		{"fails after all retries", []docker.RunOutcome{fail}, 2, store.ScheduledTaskStatusFailed, 3, []time.Duration{2 * time.Second, 4 * time.Second}},
		{"recovers on second attempt", []docker.RunOutcome{fail, ok}, 3, store.ScheduledTaskStatusSuccess, 2, []time.Duration{2 * time.Second}},
		{"no retries configured", []docker.RunOutcome{fail}, 0, store.ScheduledTaskStatusFailed, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &nativeRuntime{fakeRuntime: &fakeRuntime{}, outcomes: tt.outcomes}
			env := newJobEnv(t, rt, fakeSpecs{}, func(j *store.ScheduledTask) {
				j.MaxRetries, j.RetryBackoffSeconds = tt.retries, 2
			})
			if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
				t.Fatalf("RunTrigger: %v", err)
			}
			run := env.lastRun(t)
			if run.Status != tt.wantStatus || run.Attempts != tt.wantAttempts {
				t.Errorf("status=%q attempts=%d, want %q and %d", run.Status, run.Attempts, tt.wantStatus, tt.wantAttempts)
			}
			if len(env.sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleeps = %v, want %v", env.sleeps, tt.wantSleeps)
			}
			for i := range tt.wantSleeps {
				if env.sleeps[i] != tt.wantSleeps[i] {
					t.Errorf("sleep %d = %s, want %s", i, env.sleeps[i], tt.wantSleeps[i])
				}
			}
			task, _ := env.db.GetScheduledTask(context.Background(), "st_1")
			wantFailures := 0
			if tt.wantStatus != store.ScheduledTaskStatusSuccess {
				wantFailures = 1
			}
			if task.ConsecutiveFailures != wantFailures {
				t.Errorf("ConsecutiveFailures = %d, want %d (counted once per run, not per attempt)", task.ConsecutiveFailures, wantFailures)
			}
		})
	}
}

func TestRunner_Fresh_HalfSucceeded_StartFailsRemovesContainer(t *testing.T) {
	rt := &pollRuntime{fakeRuntime: &fakeRuntime{}, startErr: errors.New("oci runtime: exec format error")}
	env := newJobEnv(t, rt, fakeSpecs{}, nil)

	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusFailed || !strings.Contains(run.Message, "exec format error") {
		t.Errorf("run = status %q message %q, want failed with the start error", run.Status, run.Message)
	}
	if len(rt.removed) != 1 || rt.removed[0] != "job-container" {
		t.Errorf("removed = %v, want the created container force-removed so nothing is left behind", rt.removed)
	}
}

func TestRunner_Fresh_TimeoutKillsAndRemovesContainer(t *testing.T) {
	rt := &pollRuntime{fakeRuntime: &fakeRuntime{}, running: true}
	env := newJobEnv(t, rt, fakeSpecs{}, nil)
	env.runner.Timeout = 50 * time.Millisecond

	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusTimeout {
		t.Errorf("status = %q, want timeout", run.Status)
	}
	if len(rt.stopped) == 0 || len(rt.removed) == 0 {
		t.Errorf("stopped=%v removed=%v, want the timed-out container stopped and removed", rt.stopped, rt.removed)
	}
}

func TestRunner_Fresh_NonzeroExitOnRemoteNodeRecordsExitCode(t *testing.T) {
	rt := &pollRuntime{fakeRuntime: &fakeRuntime{}, exitCode: 7}
	env := newJobEnv(t, rt, fakeSpecs{}, nil)
	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	// the poll loop ticks once a second, so this waits one real tick
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusFailed || run.ExitCode == nil || *run.ExitCode != 7 {
		t.Errorf("run = %+v, want failed with exit code 7", run)
	}
}

func TestRunner_PerAppConcurrencyCap(t *testing.T) {
	hold := make(chan struct{})
	rt := &nativeRuntime{fakeRuntime: &fakeRuntime{}, hold: hold, outcomes: []docker.RunOutcome{{OutputCaptured: true}}}
	env := newJobEnv(t, rt, fakeSpecs{}, nil)
	env.runner.Limits = DefaultLimits()
	env.runner.Limits.MaxConcurrentPerApp = 1
	second := saveJob(t, env.db, "st_2", "0 4 * * *", nil)

	done := make(chan error, 1)
	go func() {
		done <- env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual})
	}()
	deadline := time.After(5 * time.Second)
	for {
		rt.mu.Lock()
		n := len(rt.specs)
		rt.mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first run never reached the runtime")
		case <-time.After(5 * time.Millisecond):
		}
	}

	if err := env.runner.RunTrigger(context.Background(), second, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("second RunTrigger: %v", err)
	}
	task, _ := env.db.GetScheduledTask(context.Background(), "st_2")
	if task.LastRunStatus != store.ScheduledTaskStatusSkippedConcurrency {
		t.Errorf("second job status = %q, want skipped_concurrency while the app is at its cap", task.LastRunStatus)
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatalf("first RunTrigger: %v", err)
	}
}

func TestRunner_Fresh_SpecBuildFailureIsRecorded(t *testing.T) {
	env := newJobEnv(t, &nativeRuntime{fakeRuntime: &fakeRuntime{}}, fakeSpecs{err: errors.New("secret \"API_KEY\" is required")}, nil)
	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusFailed || !strings.Contains(run.Message, "API_KEY") {
		t.Errorf("run = %+v, want failed naming the missing secret", run)
	}
}

func TestRunner_ExecMode_WritesHistory(t *testing.T) {
	rt := &fakeRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt.execReader = nopReader("done\n")
	env := newJobEnv(t, rt, nil, func(j *store.ScheduledTask) { j.Mode = store.ScheduledTaskModeExec })
	if err := env.runner.RunTrigger(context.Background(), env.task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	run := env.lastRun(t)
	if run.Status != store.ScheduledTaskStatusSuccess || run.Mode != store.ScheduledTaskModeExec || run.Output != "done\n" || run.ContainerID != "c1" {
		t.Errorf("run = %+v, want a successful exec-mode row", run)
	}
}

func nopReader(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }
