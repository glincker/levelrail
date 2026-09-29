package scheduledeploy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduleStore is Scheduler's own fake for ScheduleStore, the same
// "struct literal a test configures directly" shape
// internal/scheduledtask's own fakeScheduleStore establishes. Unlike
// that fake, ArmAppScheduleNextRun mutates the stored schedule in place,
// so a fresh Scheduler built against the same fakeScheduleStore (the
// "process restarted" simulation every catch-up test below uses) sees
// exactly what a real restart would see: whatever was last persisted,
// nothing kept only in the old Scheduler's memory.
type fakeScheduleStore struct {
	mu        sync.Mutex
	schedules map[string]store.AppSchedule
	history   []store.AppScheduleHistoryEntry
	listErr   error
}

func newFakeScheduleStore(schedules ...store.AppSchedule) *fakeScheduleStore {
	f := &fakeScheduleStore{schedules: map[string]store.AppSchedule{}}
	for _, s := range schedules {
		f.schedules[s.ServiceName] = s
	}
	return f
}

func (f *fakeScheduleStore) ListEnabledAppSchedules(context.Context) ([]store.AppSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []store.AppSchedule
	for _, s := range f.schedules {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeScheduleStore) ArmAppScheduleNextRun(_ context.Context, serviceName string, next time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.schedules[serviceName]
	if !ok {
		return nil
	}
	t := next
	s.NextFireAt = &t
	f.schedules[serviceName] = s
	return nil
}

func (f *fakeScheduleStore) RecordAppScheduleHistory(_ context.Context, e store.AppScheduleHistoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = append(f.history, e)
	return nil
}

func (f *fakeScheduleStore) historyFor(serviceName string) []store.AppScheduleHistoryEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.AppScheduleHistoryEntry
	for _, e := range f.history {
		if e.ServiceName == serviceName {
			out = append(out, e)
		}
	}
	return out
}

// fakeFreezeStore is Scheduler's own fake for deploy.FreezeStore.
type fakeFreezeStore struct {
	windows []store.DeployFreezeWindow
}

func (f *fakeFreezeStore) ListDeployFreezeWindows(context.Context, ...string) ([]store.DeployFreezeWindow, error) {
	return f.windows, nil
}

// fakeTrigger is Scheduler's own fake for Trigger: records every call,
// and can be told to fail for a specific service.
type fakeTrigger struct {
	mu      sync.Mutex
	calls   []call
	failFor map[string]error
}

type call struct {
	serviceName, branch string
}

func (f *fakeTrigger) TriggerScheduledDeploy(_ context.Context, serviceName, branch string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{serviceName, branch})
	if f.failFor != nil {
		if err, ok := f.failFor[serviceName]; ok {
			return "", err
		}
	}
	return "deploy triggered", nil
}

func (f *fakeTrigger) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func appSchedule(name string) store.AppSchedule {
	return store.AppSchedule{ServiceName: name, Cron: "0 3 * * *", Branch: "main", Timezone: "UTC", Enabled: true}
}

// TestScheduler_Tick_FirstSightingArmsWithoutFiring mirrors
// internal/scheduledtask's own test of the identical name: a schedule
// observed for the first time must arm its next-run time without firing.
func TestScheduler_Tick_FirstSightingArmsWithoutFiring(t *testing.T) {
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{}
	s := NewScheduler(fakeStore, nil, trigger, nil)
	s.Now = func() time.Time { return now }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if trigger.count() != 0 {
		t.Fatalf("first tick fired %d times, want 0 (arm only)", trigger.count())
	}
	if fakeStore.schedules["web"].NextFireAt == nil {
		t.Fatal("first tick did not arm NextFireAt")
	}
}

func TestScheduler_Tick_FiresOnceDueThenNotAgainSameTarget(t *testing.T) {
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{}
	s := NewScheduler(fakeStore, nil, trigger, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	mustTick(t, s)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	mustTick(t, s)
	if trigger.count() != 1 {
		t.Fatalf("due tick fired %d times, want 1", trigger.count())
	}

	// A repeat tick at the same due time must not fire again: Tick armed
	// the next occurrence before calling Trigger.
	mustTick(t, s)
	if trigger.count() != 1 {
		t.Fatalf("repeat tick at the same due time fired again, calls = %d, want 1", trigger.count())
	}
}

// TestScheduler_CatchUp_MissedOccurrencesFireAtMostOnce proves the exact
// catch-up semantics this package's own doc comment documents: several
// occurrences missed while nothing checked in still produce exactly one
// firing, not one per missed occurrence, and a brand-new Scheduler value
// (simulating a control plane restart) sees this correctly because the
// armed time is read from fakeScheduleStore, never kept in Scheduler's
// own memory.
func TestScheduler_CatchUp_MissedOccurrencesFireAtMostOnce(t *testing.T) {
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{}

	s1 := NewScheduler(fakeStore, nil, trigger, nil)
	s1.Now = func() time.Time { return time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC) }
	mustTick(t, s1)
	if trigger.count() != 0 {
		t.Fatalf("arm tick fired, want 0")
	}

	// Five days pass with no ticks at all: five occurrences of "daily at
	// 03:00" were missed (Aug 10, 11, 12, 13, 14 03:00 UTC). A brand-new
	// Scheduler value, pointed at the same store, represents the process
	// restarting.
	s2 := NewScheduler(fakeStore, nil, trigger, nil)
	s2.Now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }
	mustTick(t, s2)

	if trigger.count() != 1 {
		t.Fatalf("catch-up tick fired %d times, want exactly 1 for 5 missed occurrences", trigger.count())
	}

	// It must also not fire again on the next tick: it re-armed to the
	// next real occurrence strictly after "now" (Aug 16 03:00), not to
	// the last missed target.
	next := fakeStore.schedules["web"].NextFireAt
	if next == nil || !next.After(s2.Now()) {
		t.Fatalf("NextFireAt = %v, want a time strictly after %v", next, s2.Now())
	}
	mustTick(t, s2)
	if trigger.count() != 1 {
		t.Fatalf("tick after catch-up fired again, calls = %d, want 1", trigger.count())
	}
}

// TestScheduler_Restart_ArmedBeforeTriggerNeverDoubleFires simulates the
// half-succeeded case: the control plane restarts in the narrow window
// after Tick has persisted the next armed time but conceptually "before"
// Trigger's effect would have been observed. Because arming happens
// before Trigger is called, the persisted state alone (not a completed
// Trigger call) determines whether the next tick considers the schedule
// due again. A second Scheduler sharing the same store must not re-fire
// for the occurrence the first one already claimed.
func TestScheduler_Restart_ArmedBeforeTriggerNeverDoubleFires(t *testing.T) {
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{}

	s1 := NewScheduler(fakeStore, nil, trigger, nil)
	s1.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	mustTick(t, s1)
	s1.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	mustTick(t, s1)
	if trigger.count() != 1 {
		t.Fatalf("first scheduler fired %d times, want 1", trigger.count())
	}

	// "Restart": a new Scheduler, same store, same moment in time.
	s2 := NewScheduler(fakeStore, nil, trigger, nil)
	s2.Now = s1.Now
	mustTick(t, s2)
	if trigger.count() != 1 {
		t.Fatalf("scheduler after restart fired again at the same due time, calls = %d, want 1", trigger.count())
	}
}

func TestScheduler_Tick_FreezeWindowSkipsAndRecordsReason(t *testing.T) {
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{}
	freeze := &fakeFreezeStore{windows: []store.DeployFreezeWindow{
		{ID: "fw_1", Scope: store.DeployFreezeScopeApp("web"), Cron: "0 0 * * *", Duration: 24 * time.Hour, Reason: "launch week"},
	}}
	s := NewScheduler(fakeStore, freeze, trigger, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	mustTick(t, s)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	mustTick(t, s)

	if trigger.count() != 0 {
		t.Fatalf("frozen tick called Trigger %d times, want 0", trigger.count())
	}
	hist := fakeStore.historyFor("web")
	if len(hist) != 1 || hist[0].Status != store.AppScheduleHistorySkippedFreeze {
		t.Fatalf("history = %+v, want exactly one skipped_freeze entry", hist)
	}
	if hist[0].Reason == "" {
		t.Fatal("skipped_freeze entry recorded no reason")
	}
}

func TestScheduler_Tick_TriggerFailureRecordsFailedAndPropagates(t *testing.T) {
	fakeStore := newFakeScheduleStore(appSchedule("web"))
	trigger := &fakeTrigger{failFor: map[string]error{"web": errors.New("no git source connected")}}
	s := NewScheduler(fakeStore, nil, trigger, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	mustTick(t, s)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want the trigger failure wrapped and returned")
	}

	hist := fakeStore.historyFor("web")
	if len(hist) != 1 || hist[0].Status != store.AppScheduleHistoryFailed {
		t.Fatalf("history = %+v, want exactly one failed entry", hist)
	}

	// It must still not double-fire on the next tick: the failure is not
	// retried immediately, only at the schedule's next natural occurrence.
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("repeat tick error = %v, want nil (already re-armed)", err)
	}
	if trigger.count() != 1 {
		t.Fatalf("trigger called %d times, want 1", trigger.count())
	}
}

func TestScheduler_Tick_InvalidScheduleSkippedNotFatal(t *testing.T) {
	fakeStore := newFakeScheduleStore(
		store.AppSchedule{ServiceName: "bad", Cron: "not a cron expr", Branch: "main", Timezone: "UTC", Enabled: true},
		appSchedule("good"),
	)
	trigger := &fakeTrigger{}
	s := NewScheduler(fakeStore, nil, trigger, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (invalid schedule is skipped, not fatal)", err)
	}
	if trigger.count() != 0 {
		t.Fatalf("arm tick fired %d times, want 0", trigger.count())
	}
	if fakeStore.schedules["good"].NextFireAt == nil {
		t.Fatal("valid sibling schedule was not armed despite the invalid one")
	}
}

func mustTick(t *testing.T, s *Scheduler) {
	t.Helper()
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
}
