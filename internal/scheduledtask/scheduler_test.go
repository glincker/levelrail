package scheduledtask

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduleStore is Scheduler's own fake ScheduleStore.
type fakeScheduleStore struct {
	mu      sync.Mutex
	tasks   []store.ScheduledTask
	listErr error
}

func (f *fakeScheduleStore) ListEnabledScheduledTasks(context.Context) ([]store.ScheduledTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tasks, nil
}

// fakeTaskRunner is Scheduler's own fake TaskRunner: records every call,
// and can be told to fail always or for specific task IDs.
type fakeTaskRunner struct {
	mu    sync.Mutex
	calls []store.ScheduledTask
	err   error
}

func (f *fakeTaskRunner) Run(_ context.Context, _ string, task store.ScheduledTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, task)
	return f.err
}

func schedTask(id, schedule string) store.ScheduledTask {
	return store.ScheduledTask{ID: id, ServiceName: "web", Name: "cron", Command: "true", Schedule: schedule, Enabled: true}
}

// TestScheduler_Tick_FirstSightingArmsWithoutFiring mirrors
// internal/backup's identical test for its own Scheduler: the first tick
// a task is observed must arm its next fire time without running.
func TestScheduler_Tick_FirstSightingArmsWithoutFiring(t *testing.T) {
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{schedTask("t1", "0 3 * * *")}}
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
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{schedTask("t1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)

	armAt := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return armAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}

	dueAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return dueAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("due tick error = %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].ID != "t1" {
		t.Fatalf("due tick calls = %+v, want exactly one call for t1", runner.calls)
	}

	// A repeat tick at the same due time must not fire again.
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("repeat tick error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("repeat tick at the same due time fired again, calls = %d, want 1", len(runner.calls))
	}
}

func TestScheduler_Tick_NotYetDueDoesNotFire(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{schedTask("t1", "0 3 * * *")}}
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

// TestScheduler_Tick_RunFailurePropagates: Runner.Run failing must
// surface as a Tick error, mirroring backup.Scheduler's identical
// TestScheduler_Tick_FailedRunSkipsPrune.
func TestScheduler_Tick_RunFailurePropagates(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{schedTask("t1", "0 3 * * *")}}
	runner := &fakeTaskRunner{err: errors.New("command exited 1")}
	s := NewScheduler(fakeStore, runner, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want a wrapped Run failure")
	}
}

// TestScheduler_Tick_InvalidScheduleSkipsWithoutFailingTick: an
// unparseable cron string must be skipped, not stop evaluation of other
// tasks in the same tick.
func TestScheduler_Tick_InvalidScheduleSkipsWithoutFailingTick(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{
		schedTask("broken", "not a cron string"),
		schedTask("ok", "0 3 * * *"),
	}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (invalid schedules are skipped, not fatal)", err)
	}
	s.mu.Lock()
	_, armed := s.nextRun["ok"]
	_, brokenArmed := s.nextRun["broken"]
	s.mu.Unlock()
	if !armed {
		t.Error("valid schedule 'ok' was not armed after a tick containing an invalid schedule earlier in the list")
	}
	if brokenArmed {
		t.Error("invalid schedule 'broken' should never be armed")
	}
}

// TestScheduler_Tick_ForgetsDisabledTask: once a task drops out of
// ListEnabledScheduledTasks (disabled, deleted), Scheduler must forget
// its armed nextRun.
func TestScheduler_Tick_ForgetsDisabledTask(t *testing.T) {
	fakeStore := &fakeScheduleStore{tasks: []store.ScheduledTask{schedTask("t1", "0 3 * * *")}}
	runner := &fakeTaskRunner{}
	s := NewScheduler(fakeStore, runner, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	s.mu.Lock()
	_, armed := s.nextRun["t1"]
	s.mu.Unlock()
	if !armed {
		t.Fatal("expected 't1' to be armed after its first tick")
	}

	fakeStore.mu.Lock()
	fakeStore.tasks = nil
	fakeStore.mu.Unlock()
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	s.mu.Lock()
	_, stillArmed := s.nextRun["t1"]
	s.mu.Unlock()
	if stillArmed {
		t.Error("'t1' should have been forgotten once it dropped out of ListEnabledScheduledTasks")
	}
}

func TestScheduler_Tick_ListErrorPropagates(t *testing.T) {
	fakeStore := &fakeScheduleStore{listErr: errors.New("db unavailable")}
	s := NewScheduler(fakeStore, &fakeTaskRunner{}, nil)

	if err := s.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want the store's list error wrapped")
	}
}

func TestScheduler_Run_StopsOnContextCancel(t *testing.T) {
	fakeStore := &fakeScheduleStore{}
	s := NewScheduler(fakeStore, &fakeTaskRunner{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, time.Millisecond) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
