package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToScheduledTask(t *testing.T) {
	off := false
	got, err := toScheduledTask(spec.Job{
		Name: "report", Schedule: "0 8 * * 1", Command: []string{"sh", "-c", "report"}, Timezone: "Europe/Berlin",
		Concurrency: "replace", Timeout: "2m", Retries: 3, RetryBackoff: "15s", CatchUp: "run-once", CatchUpLookback: "30m",
		CPU: 0.25, Memory: "128Mi", WarnAfter: "1h", Enabled: &off,
	})
	if err != nil {
		t.Fatalf("toScheduledTask: %v", err)
	}
	want := store.ScheduledTask{
		Name: "report", Command: []string{"sh", "-c", "report"}, Schedule: "0 8 * * 1", Timezone: "Europe/Berlin",
		Mode: store.ScheduledTaskModeFresh, Enabled: false, ConcurrencyPolicy: "replace",
		TimeoutSeconds: 120, MaxRetries: 3, RetryBackoffSeconds: 15, CatchUp: store.ScheduledTaskCatchUpRunOnce,
		CatchUpLookbackSeconds: 1800, CPULimitMillis: 250, MemoryLimitMB: 128, WarnAfterSeconds: 3600,
	}
	if got.Name != want.Name || got.Schedule != want.Schedule || got.Timezone != want.Timezone || got.Mode != want.Mode ||
		got.Enabled != want.Enabled || got.ConcurrencyPolicy != want.ConcurrencyPolicy || got.TimeoutSeconds != want.TimeoutSeconds ||
		got.MaxRetries != want.MaxRetries || got.RetryBackoffSeconds != want.RetryBackoffSeconds || got.CatchUp != want.CatchUp ||
		got.CatchUpLookbackSeconds != want.CatchUpLookbackSeconds || got.CPULimitMillis != want.CPULimitMillis ||
		got.MemoryLimitMB != want.MemoryLimitMB || got.WarnAfterSeconds != want.WarnAfterSeconds {
		t.Errorf("toScheduledTask = %+v, want %+v", got, want)
	}

	def, _ := toScheduledTask(spec.Job{Name: "x", Schedule: "* * * * *", Command: []string{"x"}})
	if !def.Enabled || def.ConcurrencyPolicy != store.ScheduledTaskConcurrencyForbid || def.CatchUp != store.ScheduledTaskCatchUpSkip || def.Mode != store.ScheduledTaskModeFresh {
		t.Errorf("defaults = %+v, want enabled, forbid, skip, fresh", def)
	}
}

type fakeJobStore struct {
	service string
	jobs    []store.ScheduledTask
	calls   int
}

func (f *fakeJobStore) SyncSpecScheduledTasks(_ context.Context, service string, jobs []store.ScheduledTask, _ time.Time, _ func() (string, error)) error {
	f.calls++
	f.service, f.jobs = service, jobs
	return nil
}

func TestSaveJobs_NilListLeavesJobsAlone(t *testing.T) {
	js := &fakeJobStore{}
	p := &Pipeline{jobs: js}

	if err := p.saveJobs(context.Background(), Request{ServiceName: "web", Service: spec.Service{}}); err != nil || js.calls != 0 {
		t.Fatalf("nil jobs: err=%v calls=%d, want no sync", err, js.calls)
	}
	req := Request{ServiceName: "web", Service: spec.Service{Jobs: []spec.Job{}}}
	if err := p.saveJobs(context.Background(), req); err != nil || js.calls != 1 || len(js.jobs) != 0 {
		t.Fatalf("empty jobs: err=%v calls=%d jobs=%v, want one sync clearing spec jobs", err, js.calls, js.jobs)
	}
}
