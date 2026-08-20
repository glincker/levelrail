package scheduledtask

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduleStore is Scheduler's own fake for ScheduleStore, the same
// "struct literal a test configures directly, no mocking framework"
// shape internal/backup's own fakeScheduleStore establishes.
type fakeScheduleStore struct {
	tasks   []store.ScheduledTask
	listErr error
}

func (f *fakeScheduleStore) ListEnabledScheduledTasks(context.Context) ([]store.ScheduledTask, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tasks, nil
}

// fakeTaskRunner is Scheduler's own fake for TaskRunner: records every
// call, and can be told to fail either always or for specific task IDs.
type fakeTaskRunner struct {
	mu      sync.Mutex
	calls   []store.ScheduledTask
	failFor map[string]error
}

func (f *fakeTaskRunner) Run(_ context.Context, task store.ScheduledTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, task)
	if f.failFor != nil {
		if err, ok := f.failFor[task.ID]; ok {
			return err
		}
	}
	return nil
}

func scheduledTask(id, schedule string) store.ScheduledTask {
	return store.ScheduledTask{
		ID: id, ServiceName: "web", Command: []string{"echo", "hi"},
		Schedule: schedule, Enabled: true,
	}
}

// TestScheduler_Tick_FirstSightingArmsWithoutFiring mirrors
// internal/backup's own scheduler test of the identical name: the first
// tick a task is observed must arm its next-run time without firing.
func TestScheduler_Tick_FirstSightingArmsWithoutFiring(t *testing.T) {
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{scheduledTask("st_1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)
	s.Now = func() time.Time { return now }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("first tick ran %d tasks, want 0 (arm only)", len(runner.calls))
	}
}

func TestScheduler_Tick_FiresOnceDue(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{scheduledTask("st_1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("due tick error = %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].ID != "st_1" {
		t.Fatalf("due tick ran %+v, want exactly [st_1]", runner.calls)
	}

	// A third tick at the same due time must not fire again: Tick armed
	// the next occurrence before running.
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("repeat tick error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("repeat tick at the same due time ran again, calls = %d, want 1", len(runner.calls))
	}
}

func TestScheduler_Tick_NotYetDueDoesNotFire(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{scheduledTask("st_1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 2, 59, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("not-yet-due tick error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("fired before the schedule was due, calls = %d, want 0", len(runner.calls))
	}
}

// TestScheduler_Tick_InvalidScheduleSkippedNotFatal proves a task whose
// stored schedule fails to parse is logged and skipped, never ends the
// whole tick, matching backup.Scheduler's own "log and skip" handling.
func TestScheduler_Tick_InvalidScheduleSkippedNotFatal(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{
		scheduledTask("st_bad", "not a cron expr"),
		scheduledTask("st_good", "0 3 * * *"),
	}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (invalid schedule is skipped, not fatal)", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("arm tick ran %d tasks, want 0", len(runner.calls))
	}
}

// TestScheduler_Tick_OneTaskFailingDoesNotBlockOthers proves the "one
// broken task must not block the rest" guarantee: two due tasks, one
// whose Runner.Run fails, both still get a Run call and the failure
// comes back joined as Tick's own error.
func TestScheduler_Tick_OneTaskFailingDoesNotBlockOthers(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{
		scheduledTask("st_fail", "0 3 * * *"),
		scheduledTask("st_ok", "0 3 * * *"),
	}}
	failure := errors.New("boom")
	runner := &fakeTaskRunner{failFor: map[string]error{"st_fail": failure}}
	s := NewScheduler(fakeStore, runner, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	err := s.Tick(context.Background())

	if err == nil {
		t.Fatal("Tick() error = nil, want the joined failure from st_fail")
	}
	if !errors.Is(err, failure) {
		t.Errorf("Tick() error = %v, want it to wrap %v", err, failure)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("Run calls = %d, want 2 (both tasks still ran)", len(runner.calls))
	}
}

// TestScheduler_Tick_ForgetsDisabledOrDeletedTask proves a task that
// drops out of ListEnabledScheduledTasks (disabled or deleted) has its
// armed nextRun state forgotten, so re-enabling it later doesn't
// silently inherit a stale armed time.
func TestScheduler_Tick_ForgetsDisabledOrDeletedTask(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{scheduledTask("st_1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}
	if _, ok := s.nextRun["st_1"]; !ok {
		t.Fatal("st_1 not armed after first tick")
	}

	fakeStore.tasks = nil
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("empty tick error = %v", err)
	}
	if _, ok := s.nextRun["st_1"]; ok {
		t.Fatal("st_1 still armed after dropping out of the enabled set, want forgotten")
	}
}
