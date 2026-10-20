package scheduledtask

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	liveService = "lrjob-live"
	livePrefix  = "lrjobtest"
	liveSecret  = "tok-live-secret-123"
)

// liveSpecs builds job containers through the real application.Controller,
// the same code path the control plane uses.
type liveSpecs struct {
	db *store.DB
	rt docker.Runtime
}

func (l liveSpecs) BuildJobSpec(ctx context.Context, svc *store.DesiredService, _ docker.Runtime, name string, command []string, res *docker.Resources) (application.JobSpec, error) {
	return application.New(svc.Name, l.db, l.rt).BuildJobSpec(ctx, svc, name, command, res)
}

func (l liveSpecs) JobSecretValues(ctx context.Context, svc *store.DesiredService, _ docker.Runtime) ([]string, error) {
	return application.New(svc.Name, l.db, l.rt).JobSecretValues(ctx, svc)
}

func liveEnv(t *testing.T) (*docker.Client, *store.DB, *Runner) {
	t.Helper()
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.ListByPrefix(pingCtx, livePrefix); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		left, _ := client.ListByPrefix(ctx, livePrefix)
		for _, c := range left {
			_ = client.Remove(ctx, c.ID, true)
		}
		_ = client.Close()
	})

	db := openDB(t)
	svc := store.DesiredService{
		Name: liveService, Image: "alpine:latest",
		Env: map[string]string{"JOB_API_TOKEN": liveSecret, "PLAIN": "visible"},
	}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("save service: %v", err)
	}
	runner := &Runner{
		Apps: db, Runs: db, History: db, Specs: liveSpecs{db: db, rt: client}, NamePrefix: livePrefix,
		Resolver: func(string) (docker.Runtime, error) { return client, nil },
		Now:      func() time.Time { return time.Date(2026, 8, 15, 10, 0, 10, 0, time.UTC) },
	}
	return client, db, runner
}

func saveLiveJob(t *testing.T, db *store.DB, id, schedule string, command []string, mut func(*store.ScheduledTask)) store.ScheduledTask {
	t.Helper()
	task := saveJob(t, db, id, schedule, func(j *store.ScheduledTask) {
		j.ServiceName = liveService
		j.Command = command
		j.CreatedAt = time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
		if mut != nil {
			mut(j)
		}
	})
	return task
}

func runsOf(t *testing.T, db *store.DB, taskID string) []store.ScheduledTaskRun {
	t.Helper()
	runs, err := db.ListScheduledTaskRuns(context.Background(), store.ScheduledTaskRunFilter{ServiceName: liveService, TaskID: taskID})
	if err != nil {
		t.Fatalf("ListScheduledTaskRuns: %v", err)
	}
	return runs
}

func TestLive_EveryMinuteJobRunsOnceCapturesLogsAndRedactsSecrets(t *testing.T) {
	client, db, runner := liveEnv(t)
	ctx := context.Background()
	saveLiveJob(t, db, "st_live_ok", "* * * * *",
		[]string{"sh", "-c", `echo "plain=$PLAIN token=$JOB_API_TOKEN"; echo to-stderr >&2`}, nil)

	now := time.Date(2026, 8, 15, 10, 0, 10, 0, time.UTC)
	sched := NewScheduler(db, runner, nil)
	sched.Now = func() time.Time { return now }
	sched.Grace = 2 * time.Minute
	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	sched.Wait()

	runs := runsOf(t, db, "st_live_ok")
	if len(runs) != 1 {
		t.Fatalf("runs after one tick = %d, want exactly 1", len(runs))
	}
	full, err := db.GetScheduledTaskRun(ctx, runs[0].ID)
	if err != nil {
		t.Fatalf("GetScheduledTaskRun: %v", err)
	}
	if full.Status != store.ScheduledTaskStatusSuccess || full.Trigger != store.ScheduledTaskTriggerSchedule || full.Mode != store.ScheduledTaskModeFresh {
		t.Errorf("run = status %q trigger %q mode %q, want a successful scheduled fresh run (message %q)", full.Status, full.Trigger, full.Mode, full.Message)
	}
	if full.ExitCode == nil || *full.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", full.ExitCode)
	}
	if !strings.Contains(full.Output, "plain=visible") || !strings.Contains(full.Output, "to-stderr") {
		t.Errorf("Output = %q, want stdout and stderr captured", full.Output)
	}
	if strings.Contains(full.Output, liveSecret) || !strings.Contains(full.Output, redactedMarker) {
		t.Errorf("Output = %q, want the token redacted", full.Output)
	}

	if err := sched.Tick(ctx); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	restarted := NewScheduler(db, runner, nil)
	restarted.Now, restarted.Grace = func() time.Time { return now.Add(20 * time.Second) }, 2*time.Minute
	if err := restarted.Tick(ctx); err != nil {
		t.Fatalf("restarted Tick: %v", err)
	}
	sched.Wait()
	restarted.Wait()
	if got := len(runsOf(t, db, "st_live_ok")); got != 1 {
		t.Errorf("runs after a repeat tick and a restart inside the same minute = %d, want 1", got)
	}

	left, _ := client.ListByPrefix(ctx, livePrefix)
	if len(left) != 0 {
		t.Errorf("%d job containers left behind, want 0", len(left))
	}
}

func TestLive_FailingJobRetriesThenAlertsViaFailureCounter(t *testing.T) {
	_, db, runner := liveEnv(t)
	ctx := context.Background()
	task := saveLiveJob(t, db, "st_live_fail", "0 3 * * *", []string{"sh", "-c", "echo failing-now; exit 7"}, func(j *store.ScheduledTask) {
		j.MaxRetries, j.RetryBackoffSeconds = 1, 1
	})

	if err := runner.RunTrigger(ctx, task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	runs := runsOf(t, db, "st_live_fail")
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if r := runs[0]; r.Status != store.ScheduledTaskStatusFailed || r.Attempts != 2 || r.ExitCode == nil || *r.ExitCode != 7 {
		t.Errorf("run = status %q attempts %d exit %v, want failed after 2 attempts with exit 7", r.Status, r.Attempts, r.ExitCode)
	}
	got, _ := db.GetScheduledTask(ctx, "st_live_fail")
	if got.ConsecutiveFailures != 1 || got.LastRunStatus != store.ScheduledTaskStatusFailed {
		t.Errorf("task = failures %d status %q, want 1 and failed (what a scheduled_task_failure rule fires on)", got.ConsecutiveFailures, got.LastRunStatus)
	}
}

func TestLive_TimeoutKillsAndRemovesContainer(t *testing.T) {
	client, db, runner := liveEnv(t)
	ctx := context.Background()
	task := saveLiveJob(t, db, "st_live_slow", "0 3 * * *", []string{"sleep", "60"}, func(j *store.ScheduledTask) {
		j.TimeoutSeconds = 2
	})

	started := time.Now()
	if err := runner.RunTrigger(ctx, task, Trigger{Kind: store.ScheduledTaskTriggerManual}); err != nil {
		t.Fatalf("RunTrigger: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("run took %s, want it cut off near its 2s timeout", elapsed)
	}
	runs := runsOf(t, db, "st_live_slow")
	if len(runs) != 1 || runs[0].Status != store.ScheduledTaskStatusTimeout {
		t.Fatalf("runs = %+v, want one timed out run", runs)
	}
	if left, _ := client.ListByPrefix(ctx, livePrefix); len(left) != 0 {
		t.Errorf("%d job containers left after a timeout, want 0", len(left))
	}
}

func TestLive_RestartMidRunDoesNotDoubleRunAndCleansUp(t *testing.T) {
	client, db, runner := liveEnv(t)
	ctx := context.Background()
	task := saveLiveJob(t, db, "st_live_mid", "* * * * *", []string{"sleep", "120"}, nil)
	occ := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	// The previous control plane claimed the occurrence, started the
	// container, then died: a running row and a live container, no process.
	runID := "jr_crashed"
	claimed, err := db.ClaimScheduledTaskRun(ctx, store.ScheduledTaskRun{
		ID: runID, TaskID: task.ID, ServiceName: liveService, Trigger: store.ScheduledTaskTriggerSchedule,
		Status: store.ScheduledTaskStatusRunning, ScheduledFor: &occ, StartedAt: occ.Add(5 * time.Second), Mode: store.ScheduledTaskModeFresh,
	})
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	svc, _ := db.GetDesiredService(ctx, liveService)
	built, err := application.New(liveService, db, client).BuildJobSpec(ctx, svc, runner.ContainerName(liveService, runID, task.ID), []string{"sleep", "120"}, nil)
	if err != nil {
		t.Fatalf("BuildJobSpec: %v", err)
	}
	id, err := client.Create(ctx, built.Spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := client.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}

	now := occ.Add(40 * time.Second)
	fresh := NewScheduler(db, runner, nil)
	fresh.Now, fresh.Grace = func() time.Time { return now }, 2*time.Minute
	if err := fresh.Recover(ctx, runner); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if err := fresh.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	fresh.Wait()

	runs := runsOf(t, db, task.ID)
	if len(runs) != 1 || runs[0].Status != store.ScheduledTaskStatusInterrupted {
		t.Fatalf("runs = %+v, want only the interrupted run, no second run of the same occurrence", runs)
	}
	if left, _ := client.ListByPrefix(ctx, livePrefix); len(left) != 0 {
		t.Errorf("%d job containers left after recovery, want the orphan removed", len(left))
	}
}
