package scheduledeploy

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var oneShotAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

func oneShotSchedule() store.AppSchedule {
	at := oneShotAt
	return store.AppSchedule{ServiceName: "web", Branch: "main", Timezone: "UTC", Enabled: true, RunAt: &at}
}

func tickAt(t *testing.T, s *Scheduler, at time.Time) {
	t.Helper()
	s.Now = func() time.Time { return at }
	mustTick(t, s)
}

func TestOneShot_NotBeforeRunAtThenFiresOnceAndCompletes(t *testing.T) {
	fs := newFakeScheduleStore(oneShotSchedule())
	trigger := &fakeTrigger{}
	s := NewScheduler(fs, nil, trigger, nil)

	tickAt(t, s, oneShotAt.Add(-time.Minute))
	if trigger.count() != 0 {
		t.Fatalf("fired before run_at: %d calls", trigger.count())
	}
	if got := fs.schedules["web"].NextFireAt; got == nil || !got.Equal(oneShotAt) {
		t.Fatalf("NextFireAt = %v, want run_at", got)
	}

	tickAt(t, s, oneShotAt.Add(30*time.Second))
	if trigger.count() != 1 {
		t.Fatalf("calls = %d, want 1", trigger.count())
	}
	got := fs.schedules["web"]
	if got.Enabled || got.CompletedAt == nil {
		t.Fatalf("schedule = %+v, want disabled and completed", got)
	}
	h := fs.historyFor("web")
	if len(h) != 1 || h[0].Status != store.AppScheduleHistoryFired || !h[0].ScheduledFor.Equal(oneShotAt) {
		t.Fatalf("history = %+v, want one fired entry for run_at", h)
	}

	tickAt(t, s, oneShotAt.Add(time.Hour))
	if trigger.count() != 1 {
		t.Fatalf("fired again after completion: %d calls", trigger.count())
	}
}

func TestOneShot_FreezeWindowSkipsAndCompletes(t *testing.T) {
	fs := newFakeScheduleStore(oneShotSchedule())
	trigger := &fakeTrigger{}
	freeze := &fakeFreezeStore{windows: []store.DeployFreezeWindow{
		{ID: "fw_1", Scope: store.DeployFreezeScopeApp("web"), Cron: "0 0 * * *", Duration: 24 * time.Hour, Reason: "launch week"},
	}}
	s := NewScheduler(fs, freeze, trigger, nil)

	tickAt(t, s, oneShotAt.Add(time.Second))
	if trigger.count() != 0 {
		t.Fatalf("frozen one-shot fired %d times", trigger.count())
	}
	h := fs.historyFor("web")
	if len(h) != 1 || h[0].Status != store.AppScheduleHistorySkippedFreeze {
		t.Fatalf("history = %+v, want one skipped_freeze entry", h)
	}
	if fs.schedules["web"].CompletedAt == nil {
		t.Fatal("frozen one-shot not completed; it must not be queued past the freeze")
	}
}

func TestOneShot_RestartCatchUpWithinWindowFiresOnce(t *testing.T) {
	fs := newFakeScheduleStore(oneShotSchedule())
	trigger := &fakeTrigger{}
	tickAt(t, NewScheduler(fs, nil, trigger, nil), oneShotAt.Add(-time.Hour))

	restarted := NewScheduler(fs, nil, trigger, nil)
	restarted.OneShotCatchUp = 6 * time.Hour
	tickAt(t, restarted, oneShotAt.Add(2*time.Hour))
	tickAt(t, restarted, oneShotAt.Add(2*time.Hour+time.Minute))
	if trigger.count() != 1 {
		t.Fatalf("calls = %d, want exactly 1 catch-up fire", trigger.count())
	}
}

func TestOneShot_MissedBeyondWindowRecordedSkipped(t *testing.T) {
	fs := newFakeScheduleStore(oneShotSchedule())
	trigger := &fakeTrigger{}
	s := NewScheduler(fs, nil, trigger, nil)
	s.OneShotCatchUp = time.Hour

	tickAt(t, s, oneShotAt.Add(5*time.Hour))
	if trigger.count() != 0 {
		t.Fatalf("fired a one-shot missed beyond the window")
	}
	h := fs.historyFor("web")
	if len(h) != 1 || h[0].Status != store.AppScheduleHistorySkippedMissed {
		t.Fatalf("history = %+v, want one skipped_missed entry", h)
	}
	if fs.schedules["web"].CompletedAt == nil {
		t.Fatal("missed one-shot not completed")
	}
}

func TestOneShot_RearmAfterUpdateFiresAgain(t *testing.T) {
	fs := newFakeScheduleStore(oneShotSchedule())
	trigger := &fakeTrigger{}
	s := NewScheduler(fs, nil, trigger, nil)
	tickAt(t, s, oneShotAt)
	if trigger.count() != 1 {
		t.Fatalf("calls = %d, want 1", trigger.count())
	}

	// What SetAppScheduleRunAt does to the row: new run_at, completed cleared, enabled.
	later := oneShotAt.Add(48 * time.Hour)
	fs.mu.Lock()
	row := fs.schedules["web"]
	row.RunAt, row.CompletedAt, row.Enabled, row.NextFireAt = &later, nil, true, &later
	fs.schedules["web"] = row
	fs.mu.Unlock()

	tickAt(t, s, later.Add(-time.Minute))
	if trigger.count() != 1 {
		t.Fatalf("re-armed one-shot fired early")
	}
	tickAt(t, s, later)
	if trigger.count() != 2 {
		t.Fatalf("calls = %d, want 2 after re-arm", trigger.count())
	}
}
