package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func seedGuardedTask(t *testing.T, db *DB, id, policy string) {
	t.Helper()
	task := newTestScheduledTask(id, "web")
	task.ConcurrencyPolicy = policy
	if err := db.SaveScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
}

func guardedClaim(ctx context.Context, db *DB, taskID, runID string, occ time.Time) (ClaimOutcome, error) {
	return db.ClaimGuardedScheduledTaskRun(ctx, ScheduledTaskRun{
		ID: runID, TaskID: taskID, ServiceName: "web", Trigger: ScheduledTaskTriggerSchedule,
		Status: ScheduledTaskStatusRunning, ScheduledFor: &occ, StartedAt: occ,
	})
}

func TestGuardedClaim_SkipStartsOnlyOneUnderConcurrency(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	seedGuardedTask(t, db, "st_1", ScheduledTaskConcurrencyForbid)

	base := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[ClaimOutcome]int{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each claimer is a separate scheduler racing on its own occurrence.
			out, err := guardedClaim(ctx, db, "st_1", fmt.Sprintf("jr_%d", i), base.Add(time.Duration(i)*time.Minute))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				counts["error"]++
				return
			}
			counts[out]++
		}(i)
	}
	wg.Wait()
	if counts[ClaimStarted] != 1 || counts[ClaimSkipped]+counts["error"] != 7 {
		t.Fatalf("outcomes = %v, want exactly one started and the rest skipped", counts)
	}
	runs, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1"})
	running := 0
	for _, r := range runs {
		if r.Status == ScheduledTaskStatusRunning {
			running++
		}
	}
	if running != 1 {
		t.Errorf("running rows = %d, want 1", running)
	}
}

func TestGuardedClaim_QueueOneHoldsOneAndPromotes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	seedGuardedTask(t, db, "st_1", ScheduledTaskConcurrencyQueueOne)
	base := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)

	want := []ClaimOutcome{ClaimStarted, ClaimQueued, ClaimSkipped, ClaimSkipped}
	for i, w := range want {
		got, err := guardedClaim(ctx, db, "st_1", fmt.Sprintf("jr_%d", i), base.Add(time.Duration(i)*time.Minute))
		if err != nil || got != w {
			t.Fatalf("claim %d = %q, %v, want %q", i, got, err, w)
		}
	}
	if got, _ := guardedClaim(ctx, db, "st_1", "jr_dup", base); got != ClaimDuplicate {
		t.Errorf("repeat occurrence = %q, want duplicate", got)
	}
	if promoted, _ := db.PromoteQueuedScheduledTaskRuns(ctx, base.Add(time.Hour), []QueuedTarget{{TaskID: "st_1", MaxWait: 24 * time.Hour}}); len(promoted) != 0 {
		t.Fatalf("promoted %d while the first run is running, want 0", len(promoted))
	}
	fin := base.Add(30 * time.Minute)
	if err := db.FinishScheduledTaskRun(ctx, ScheduledTaskRun{ID: "jr_0", TaskID: "st_1", ServiceName: "web", Status: ScheduledTaskStatusSuccess, StartedAt: base, FinishedAt: &fin}); err != nil {
		t.Fatalf("FinishScheduledTaskRun: %v", err)
	}
	promoted, err := db.PromoteQueuedScheduledTaskRuns(ctx, base.Add(time.Hour), []QueuedTarget{{TaskID: "st_1", MaxWait: 24 * time.Hour}})
	if err != nil || len(promoted) != 1 || promoted[0].ID != "jr_1" || promoted[0].Status != ScheduledTaskStatusRunning {
		t.Fatalf("promote = %+v, %v, want jr_1 running", promoted, err)
	}
	if again, _ := db.PromoteQueuedScheduledTaskRuns(ctx, base.Add(time.Hour), []QueuedTarget{{TaskID: "st_1", MaxWait: 24 * time.Hour}}); len(again) != 0 {
		t.Errorf("second promote = %d, want 0", len(again))
	}
}

func TestPruneScheduledTaskRuns_BoundedPerSweep(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		at := base.Add(time.Duration(i) * time.Hour)
		_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{
			ID: fmt.Sprintf("jr_%02d", i), TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerManual,
			Status: ScheduledTaskStatusSuccess, StartedAt: at, FinishedAt: &at,
		})
	}
	count := func() int {
		runs, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1", Limit: 100})
		return len(runs)
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", RunRetention{Keep: 5}); err != nil || count() != 20 {
		t.Fatalf("MaxRows 0 deleted rows (count %d, err %v), want none", count(), err)
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", RunRetention{Keep: 5, MaxRows: 4}); err != nil || count() != 16 {
		t.Fatalf("count after one sweep = %d (err %v), want 16", count(), err)
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", RunRetention{Before: base.Add(18 * time.Hour), MaxRows: 100}); err != nil || count() != 2 {
		t.Fatalf("count after age prune = %d (err %v), want 2", count(), err)
	}
}

func seedFailingAlertJob(t *testing.T, db *DB, t0 time.Time) {
	t.Helper()
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))
	if err := db.SetScheduledTaskGuards(ctx, "st_1", ScheduledTaskGuards{AlertOnFailure: true}); err != nil {
		t.Fatalf("SetScheduledTaskGuards: %v", err)
	}
	code := 3
	_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{ID: "jr_1", TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerManual,
		Status: ScheduledTaskStatusFailed, StartedAt: t0, FinishedAt: &t0, ExitCode: &code, Output: "SECRET_TOKEN=abc"})
	// A later skip must not overwrite the exit code the alert reports.
	later := t0.Add(time.Second)
	_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{ID: "jr_2", TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerSchedule,
		Status: ScheduledTaskStatusSkippedConcurrency, StartedAt: later, FinishedAt: &later})
	_ = db.RecordScheduledTaskRun(ctx, "st_1", t0, ScheduledTaskStatusFailed, "SECRET_TOKEN=abc")
}

func TestClaimJobFailureAlerts_RateLimitedPerRuleAndJob(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	seedFailingAlertJob(t, db, t0)

	window := time.Hour
	claim := func(rule string, now time.Time) []JobFailureAlert {
		t.Helper()
		got, err := db.ClaimJobFailureAlerts(ctx, rule, "web", now, now.Add(-window), 10)
		if err != nil {
			t.Fatalf("ClaimJobFailureAlerts: %v", err)
		}
		return got
	}
	first := claim("ar_a", t0.Add(time.Minute))
	if len(first) != 1 || first[0].ExitCode == nil || *first[0].ExitCode != 3 {
		t.Fatalf("first claim = %+v, want one alert with exit code 3", first)
	}
	if other := claim("ar_b", t0.Add(time.Minute)); len(other) != 1 {
		t.Fatalf("second rule got %d alerts, want its own 1", len(other))
	}
	if again := claim("ar_a", t0.Add(2*time.Minute)); len(again) != 0 {
		t.Fatalf("same failure claimed twice: %+v", again)
	}
	if none, _ := db.ClaimJobFailureAlerts(ctx, "ar_c", "", t0, t0, 10); len(none) != 0 {
		t.Fatalf("empty service claimed %d, want 0", len(none))
	}
	_ = db.RecordScheduledTaskRun(ctx, "st_1", t0.Add(10*time.Minute), ScheduledTaskStatusFailed, "")
	if inWindow := claim("ar_a", t0.Add(11*time.Minute)); len(inWindow) != 0 {
		t.Fatalf("new failure inside the window alerted: %+v", inWindow)
	}
	if later := claim("ar_a", t0.Add(2*time.Hour)); len(later) != 1 {
		t.Fatalf("failure after the window = %d alerts, want 1", len(later))
	}
}

func TestReleaseJobFailureAlert_RetriesAfterFailedSend(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	seedFailingAlertJob(t, db, t0)
	now := t0.Add(time.Minute)
	if got, _ := db.ClaimJobFailureAlerts(ctx, "ar_a", "web", now, now.Add(-time.Hour), 10); len(got) != 1 {
		t.Fatalf("claim = %d, want 1", len(got))
	}
	if err := db.ReleaseJobFailureAlert(ctx, "ar_a", "st_1", now); err != nil {
		t.Fatalf("ReleaseJobFailureAlert: %v", err)
	}
	retry := now.Add(time.Minute)
	if got, _ := db.ClaimJobFailureAlerts(ctx, "ar_a", "web", retry, retry.Add(-time.Hour), 10); len(got) != 1 {
		t.Fatalf("claim after a failed send = %d, want 1", len(got))
	}
}

func TestPromoteQueued_OnlyTargetsAndExpiresStale(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	seedGuardedTask(t, db, "st_1", ScheduledTaskConcurrencyQueueOne)
	base := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	_, _ = guardedClaim(ctx, db, "st_1", "jr_0", base)
	_, _ = guardedClaim(ctx, db, "st_1", "jr_1", base.Add(time.Minute))
	fin := base.Add(2 * time.Minute)
	_ = db.FinishScheduledTaskRun(ctx, ScheduledTaskRun{ID: "jr_0", TaskID: "st_1", ServiceName: "web", Status: ScheduledTaskStatusSuccess, StartedAt: base, FinishedAt: &fin})

	if got, _ := db.PromoteQueuedScheduledTaskRuns(ctx, fin, nil); len(got) != 0 {
		t.Fatalf("promote without targets started %d runs", len(got))
	}
	task, _ := db.GetScheduledTask(ctx, "st_1")
	task.Enabled = false
	_ = db.UpdateScheduledTaskConfig(ctx, task, fin)
	if got, _ := db.PromoteQueuedScheduledTaskRuns(ctx, fin, []QueuedTarget{{TaskID: "st_1", MaxWait: time.Hour}}); len(got) != 0 {
		t.Fatalf("disabled job promoted %d runs", len(got))
	}
	if run, _ := db.GetScheduledTaskRun(ctx, "jr_1"); run.Status != ScheduledTaskStatusQueued {
		t.Fatalf("queued row of a disabled job = %q, want still queued (no orphan running row)", run.Status)
	}
	task.Enabled = true
	_ = db.UpdateScheduledTaskConfig(ctx, task, fin)
	weeks := base.Add(21 * 24 * time.Hour)
	if got, _ := db.PromoteQueuedScheduledTaskRuns(ctx, weeks, []QueuedTarget{{TaskID: "st_1", MaxWait: time.Hour}}); len(got) != 0 {
		t.Fatalf("stale queued run started weeks later")
	}
	if run, _ := db.GetScheduledTaskRun(ctx, "jr_1"); run.Status != ScheduledTaskStatusMissed {
		t.Errorf("stale queued row = %q, want missed", run.Status)
	}
}

func TestPruneScheduledTaskRuns_SkippedDoNotDisplaceRealRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 13; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		status := ScheduledTaskStatusSkippedConcurrency
		if i < 3 {
			status = ScheduledTaskStatusSuccess
		}
		_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{ID: fmt.Sprintf("jr_%02d", i), TaskID: "st_1", ServiceName: "web",
			Trigger: ScheduledTaskTriggerSchedule, Status: status, StartedAt: at, FinishedAt: &at})
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", RunRetention{Keep: 3, SkippedKeep: 2, MaxRows: 100}); err != nil {
		t.Fatalf("PruneScheduledTaskRuns: %v", err)
	}
	ok, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1", Status: ScheduledTaskStatusSuccess, Limit: 100})
	skipped, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1", Status: ScheduledTaskStatusSkippedConcurrency, Limit: 100})
	if len(ok) != 3 || len(skipped) != 2 {
		t.Fatalf("after prune: %d real, %d skipped, want 3 and 2", len(ok), len(skipped))
	}
}

func TestRecreatedJobDoesNotInheritGuards(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	seedGuardedTask(t, db, "st_old", ScheduledTaskConcurrencyQueueOne)
	_ = db.SetScheduledTaskGuards(ctx, "st_old", ScheduledTaskGuards{AlertOnFailure: true, KeepRuns: 7, KeepDays: 3})
	base := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	_, _ = guardedClaim(ctx, db, "st_old", "jr_0", base)
	if got, _ := guardedClaim(ctx, db, "st_old", "jr_1", base.Add(time.Minute)); got != ClaimQueued {
		t.Fatalf("setup: second claim = %q, want queued", got)
	}
	if err := db.DeleteScheduledTask(ctx, "st_old"); err != nil {
		t.Fatalf("DeleteScheduledTask: %v", err)
	}
	if _, err := db.GetScheduledTaskRun(ctx, "jr_1"); err == nil {
		t.Error("queued run of a deleted job survived")
	}
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_new", "web"))
	g, err := db.GetScheduledTaskGuards(ctx, "st_new")
	if err != nil || g != (ScheduledTaskGuards{}) {
		t.Errorf("recreated job guards = %+v, %v, want zero", g, err)
	}
}

func TestPruneScheduledTaskRuns_FreezeSkipsDoNotDisplaceRealRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 13; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		status := ScheduledTaskStatusSkippedFreeze
		if i < 3 {
			status = ScheduledTaskStatusSuccess
		}
		_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{ID: fmt.Sprintf("jr_%02d", i), TaskID: "st_1", ServiceName: "web",
			Trigger: ScheduledTaskTriggerSchedule, Status: status, StartedAt: at, FinishedAt: &at})
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", RunRetention{Keep: 3, SkippedKeep: 2, MaxRows: 100}); err != nil {
		t.Fatalf("PruneScheduledTaskRuns: %v", err)
	}
	ok, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1", Status: ScheduledTaskStatusSuccess, Limit: 100})
	skipped, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web", TaskID: "st_1", Status: ScheduledTaskStatusSkippedFreeze, Limit: 100})
	if len(ok) != 3 || len(skipped) != 2 {
		t.Fatalf("after prune: %d real, %d freeze skips, want 3 and 2", len(ok), len(skipped))
	}
}
