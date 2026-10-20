package scheduledtask

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	return db
}

func saveJob(t *testing.T, db *store.DB, id, schedule string, mut func(*store.ScheduledTask)) store.ScheduledTask {
	t.Helper()
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	task := store.ScheduledTask{
		ID: id, ServiceName: "web", Command: []string{"echo", "hi"}, Schedule: schedule, Enabled: true,
		Mode: store.ScheduledTaskModeFresh, Timezone: "UTC", CreatedAt: created, UpdatedAt: created,
	}
	if mut != nil {
		mut(&task)
	}
	if err := db.SaveScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
	return task
}

type recordingRunner struct {
	mu      sync.Mutex
	runs    []Trigger
	tasks   []string
	block   chan struct{}
	started chan struct{}
}

func (r *recordingRunner) RunTrigger(_ context.Context, task store.ScheduledTask, trig Trigger) error {
	r.mu.Lock()
	r.runs = append(r.runs, trig)
	r.tasks = append(r.tasks, task.ID)
	r.mu.Unlock()
	if r.started != nil {
		r.started <- struct{}{}
	}
	if r.block != nil {
		<-r.block
	}
	return nil
}

func (r *recordingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func newSched(db *store.DB, runner TaskRunner, now time.Time) *Scheduler {
	s := NewScheduler(db, runner, nil)
	s.Now = func() time.Time { return now }
	s.Grace = 2 * time.Minute
	return s
}

func mustTick(t *testing.T, s *Scheduler) {
	t.Helper()
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	s.Wait()
}

func TestOccurrences_TimezoneAndDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	kolkata, _ := time.LoadLocation("Asia/Kolkata")

	tests := []struct {
		name       string
		expr       string
		loc        *time.Location
		after, to  time.Time
		wantUTC    []string
		wantCountN int
	}{
		{
			name: "utc daily", expr: "0 3 * * *", loc: time.UTC,
			after: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC),
			wantUTC: []string{"2026-08-14T03:00:00Z", "2026-08-15T03:00:00Z"},
		},
		{
			name: "kolkata offset half hour", expr: "0 9 * * *", loc: kolkata,
			after: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
			wantUTC: []string{"2026-08-14T03:30:00Z"},
		},
		{
			name: "new york before spring forward is EST", expr: "30 2 * * *", loc: ny,
			after: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 3, 8, 6, 0, 0, 0, time.UTC),
			wantUTC: []string{"2026-03-07T07:30:00Z"},
		},
		{
			name: "spring forward nonexistent 02:30 runs once", expr: "30 2 * * *", loc: ny,
			after: time.Date(2026, 3, 8, 6, 0, 0, 0, time.UTC), to: time.Date(2026, 3, 9, 6, 0, 0, 0, time.UTC),
			wantCountN: 1,
		},
		{
			name: "after spring forward is EDT", expr: "30 2 * * *", loc: ny,
			after: time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
			wantUTC: []string{"2026-03-09T06:30:00Z"},
		},
		{
			name: "fall back repeated 01:30 runs once", expr: "30 1 * * *", loc: ny,
			after: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
			wantCountN: 1,
		},
		{
			name: "every minute across an hour", expr: "* * * * *", loc: time.UTC,
			after: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC),
			wantCountN: 60,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := cronexpr.Parse(tt.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, truncated := Occurrences(sched, tt.loc, tt.after, tt.to, 10000)
			if truncated {
				t.Fatal("truncated")
			}
			if tt.wantUTC != nil {
				if len(got) != len(tt.wantUTC) {
					t.Fatalf("got %d occurrences %v, want %v", len(got), got, tt.wantUTC)
				}
				for i, w := range tt.wantUTC {
					if g := got[i].UTC().Format(time.RFC3339); g != w {
						t.Errorf("occurrence %d = %s, want %s", i, g, w)
					}
				}
			}
			if tt.wantCountN > 0 && len(got) != tt.wantCountN {
				t.Errorf("got %d occurrences %v, want %d", len(got), got, tt.wantCountN)
			}
		})
	}
}

func TestScheduler_FiresOnceAndNotTwice(t *testing.T) {
	db := openDB(t)
	saveJob(t, db, "st_1", "0 3 * * *", nil)
	runner := &recordingRunner{}

	s := newSched(db, runner, time.Date(2026, 8, 14, 3, 0, 20, 0, time.UTC))
	mustTick(t, s)
	mustTick(t, s)
	if got := runner.count(); got != 1 {
		t.Fatalf("runs after two ticks of the same occurrence = %d, want 1", got)
	}

	restarted := newSched(db, runner, time.Date(2026, 8, 14, 3, 0, 50, 0, time.UTC))
	mustTick(t, restarted)
	if got := runner.count(); got != 1 {
		t.Fatalf("runs after a restart inside the same occurrence = %d, want 1 (no double run)", got)
	}

	next := newSched(db, runner, time.Date(2026, 8, 15, 3, 0, 5, 0, time.UTC))
	mustTick(t, next)
	if got := runner.count(); got != 2 {
		t.Fatalf("runs after the next day's occurrence = %d, want 2", got)
	}
}

func TestScheduler_NewJobDoesNotBackfill(t *testing.T) {
	db := openDB(t)
	saveJob(t, db, "st_1", "0 3 * * *", func(j *store.ScheduledTask) {
		j.CreatedAt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	})
	runner := &recordingRunner{}
	mustTick(t, newSched(db, runner, time.Date(2026, 8, 14, 12, 1, 0, 0, time.UTC)))
	if got := runner.count(); got != 0 {
		t.Fatalf("a job created at noon ran %d times before its first 03:00, want 0", got)
	}
	runs, err := db.ListScheduledTaskRuns(context.Background(), store.ScheduledTaskRunFilter{ServiceName: "web"})
	if err != nil || len(runs) != 0 {
		t.Fatalf("history = %v, %v, want empty (nothing missed before the job existed)", runs, err)
	}
}

func TestScheduler_CatchUp(t *testing.T) {
	tests := []struct {
		name        string
		catchUp     string
		lookbackSec int
		downFor     time.Duration
		wantRuns    int
		wantMissed  int
	}{
		{name: "skip records a missed run", catchUp: store.ScheduledTaskCatchUpSkip, downFor: 30 * time.Minute, wantMissed: 1},
		{name: "run once inside lookback", catchUp: store.ScheduledTaskCatchUpRunOnce, lookbackSec: 3600, downFor: 30 * time.Minute, wantRuns: 1},
		{name: "run once outside lookback is missed", catchUp: store.ScheduledTaskCatchUpRunOnce, lookbackSec: 150, downFor: 30 * time.Minute, wantMissed: 1},
		{name: "default lookback applies when unset", catchUp: store.ScheduledTaskCatchUpRunOnce, downFor: 20 * time.Minute, wantRuns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openDB(t)
			saveJob(t, db, "st_1", "*/5 * * * *", func(j *store.ScheduledTask) {
				j.CatchUp, j.CatchUpLookbackSeconds = tt.catchUp, tt.lookbackSec
			})
			runner := &recordingRunner{}
			last := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
			mustTick(t, newSched(db, runner, last))
			before := runner.count()

			back := newSched(db, runner, last.Add(tt.downFor+3*time.Minute))
			mustTick(t, back)
			if got := runner.count() - before; got != tt.wantRuns {
				t.Errorf("runs after downtime = %d, want %d", got, tt.wantRuns)
			}
			missed, _ := db.ListScheduledTaskRuns(context.Background(), store.ScheduledTaskRunFilter{ServiceName: "web", Status: store.ScheduledTaskStatusMissed})
			if len(missed) != tt.wantMissed {
				t.Errorf("missed rows = %d, want %d", len(missed), tt.wantMissed)
			}
			if tt.wantRuns == 1 {
				if tr := runner.runs[len(runner.runs)-1]; !tr.CatchUp {
					t.Errorf("trigger CatchUp = false, want true for a late run")
				}
			}
			task, _ := db.GetScheduledTask(context.Background(), "st_1")
			if task.ConsecutiveMissed != tt.wantMissed {
				t.Errorf("ConsecutiveMissed = %d, want %d", task.ConsecutiveMissed, tt.wantMissed)
			}
		})
	}
}

func TestScheduler_RestartMidRunDoesNotDoubleRun(t *testing.T) {
	db := openDB(t)
	saveJob(t, db, "st_1", "*/5 * * * *", nil)
	ctx := context.Background()

	blocked := &recordingRunner{block: make(chan struct{}), started: make(chan struct{}, 1)}
	first := newSched(db, blocked, time.Date(2026, 8, 14, 10, 5, 10, 0, time.UTC))
	if err := first.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	<-blocked.started

	// The first process dies here with its run still marked running. The
	// restarted process must interrupt it, never re-run the occurrence.
	fresh := &recordingRunner{}
	second := newSched(db, fresh, time.Date(2026, 8, 14, 10, 5, 40, 0, time.UTC))
	if err := second.Recover(ctx, nil); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	mustTick(t, second)
	if got := fresh.count(); got != 0 {
		t.Fatalf("restarted scheduler re-ran the in-flight occurrence %d times, want 0", got)
	}
	runs, _ := db.ListScheduledTaskRuns(ctx, store.ScheduledTaskRunFilter{ServiceName: "web"})
	if len(runs) != 1 || runs[0].Status != store.ScheduledTaskStatusInterrupted {
		t.Fatalf("history = %+v, want exactly one interrupted run", runs)
	}
	task, _ := db.GetScheduledTask(ctx, "st_1")
	if task.RunningSince != nil || task.ConsecutiveFailures != 1 {
		t.Errorf("task after recovery: RunningSince=%v ConsecutiveFailures=%d, want nil and 1", task.RunningSince, task.ConsecutiveFailures)
	}

	later := newSched(db, fresh, time.Date(2026, 8, 14, 10, 10, 5, 0, time.UTC))
	mustTick(t, later)
	if got := fresh.count(); got != 1 {
		t.Fatalf("next occurrence runs = %d, want 1", got)
	}
	close(blocked.block)
	first.Wait()
}

func TestScheduler_InvalidScheduleOrTimezoneIsSkipped(t *testing.T) {
	db := openDB(t)
	saveJob(t, db, "st_bad_tz", "* * * * *", func(j *store.ScheduledTask) { j.Timezone = "Mars/Olympus" })
	saveJob(t, db, "st_ok", "* * * * *", nil)
	runner := &recordingRunner{}
	mustTick(t, newSched(db, runner, time.Date(2026, 8, 14, 10, 5, 10, 0, time.UTC)))
	if runner.count() != 1 || runner.tasks[0] != "st_ok" {
		t.Fatalf("ran %v, want only st_ok", runner.tasks)
	}
}

func TestScheduler_DisabledJobDoesNotRunAndReenableDoesNotBackfill(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	task := saveJob(t, db, "st_1", "* * * * *", func(j *store.ScheduledTask) { j.Enabled = false })
	runner := &recordingRunner{}
	mustTick(t, newSched(db, runner, time.Date(2026, 8, 14, 10, 5, 10, 0, time.UTC)))
	if runner.count() != 0 {
		t.Fatal("a disabled job ran")
	}
	task.Enabled = true
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if err := db.UpdateScheduledTaskConfig(ctx, task, now); err != nil {
		t.Fatalf("UpdateScheduledTaskConfig: %v", err)
	}
	mustTick(t, newSched(db, runner, now.Add(5*time.Second)))
	missed, _ := db.ListScheduledTaskRuns(ctx, store.ScheduledTaskRunFilter{ServiceName: "web", Status: store.ScheduledTaskStatusMissed})
	if len(missed) != 0 {
		t.Fatalf("re-enabling backfilled %d missed rows, want 0", len(missed))
	}
}
