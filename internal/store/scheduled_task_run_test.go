package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestClaimScheduledTaskRun_IsIdempotentPerOccurrence(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	if err := db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web")); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}

	occ := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	claim := func(id string) bool {
		t.Helper()
		ok, err := db.ClaimScheduledTaskRun(ctx, ScheduledTaskRun{
			ID: id, TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerSchedule,
			Status: ScheduledTaskStatusRunning, ScheduledFor: &occ, StartedAt: occ,
		})
		if err != nil {
			t.Fatalf("ClaimScheduledTaskRun: %v", err)
		}
		return ok
	}
	if !claim("jr_a") {
		t.Fatal("first claim = false, want true")
	}
	if claim("jr_b") {
		t.Error("second claim for the same occurrence = true, want false")
	}

	task, _ := db.GetScheduledTask(ctx, "st_1")
	if task.LastScheduledFor == nil || !task.LastScheduledFor.Equal(occ) || task.RunningSince == nil {
		t.Errorf("task = LastScheduledFor %v RunningSince %v, want the claimed occurrence and a running mark", task.LastScheduledFor, task.RunningSince)
	}
}

func TestFinishAndInterruptScheduledTaskRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))

	start := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	for _, id := range []string{"jr_done", "jr_inflight"} {
		if err := db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{
			ID: id, TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerManual,
			Status: ScheduledTaskStatusRunning, StartedAt: start, Mode: ScheduledTaskModeFresh,
		}); err != nil {
			t.Fatalf("CreateScheduledTaskRun: %v", err)
		}
	}
	end := start.Add(5 * time.Second)
	code := 0
	if err := db.FinishScheduledTaskRun(ctx, ScheduledTaskRun{
		ID: "jr_done", TaskID: "st_1", Status: ScheduledTaskStatusSuccess, FinishedAt: &end, ExitCode: &code, DurationMS: 5000, Attempts: 1, Output: "ok",
	}); err != nil {
		t.Fatalf("FinishScheduledTaskRun: %v", err)
	}
	if task, _ := db.GetScheduledTask(ctx, "st_1"); task.RunningSince == nil {
		t.Error("RunningSince cleared while another run is still in flight")
	}

	interrupted, err := db.InterruptRunningScheduledTaskRuns(ctx, end)
	if err != nil || len(interrupted) != 1 || interrupted[0].ID != "jr_inflight" {
		t.Fatalf("InterruptRunningScheduledTaskRuns = %v, %v, want only jr_inflight", interrupted, err)
	}
	got, _ := db.GetScheduledTaskRun(ctx, "jr_inflight")
	if got.Status != ScheduledTaskStatusInterrupted || got.FinishedAt == nil {
		t.Errorf("run after interrupt = %+v", got)
	}
	task, _ := db.GetScheduledTask(ctx, "st_1")
	if task.RunningSince != nil || task.ConsecutiveFailures != 1 || task.LastRunStatus != ScheduledTaskStatusInterrupted {
		t.Errorf("task after interrupt = %+v", task)
	}
}

func TestPruneScheduledTaskRuns_KeepsNewestAndBlanksOldOutput(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	_ = db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web"))
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		fin := base.Add(time.Duration(i) * time.Hour)
		_ = db.CreateScheduledTaskRun(ctx, ScheduledTaskRun{
			ID: "jr_" + string(rune('a'+i)), TaskID: "st_1", ServiceName: "web", Trigger: ScheduledTaskTriggerManual,
			Status: ScheduledTaskStatusSuccess, StartedAt: fin, FinishedAt: &fin, Output: "out",
		})
	}
	if err := db.PruneScheduledTaskRuns(ctx, "st_1", 3, base.Add(3*time.Hour)); err != nil {
		t.Fatalf("PruneScheduledTaskRuns: %v", err)
	}
	runs, _ := db.ListScheduledTaskRuns(ctx, ScheduledTaskRunFilter{ServiceName: "web"})
	if len(runs) != 3 || runs[0].ID != "jr_e" {
		t.Fatalf("runs after prune = %d (%v), want the newest 3", len(runs), runs)
	}
	oldest, _ := db.GetScheduledTaskRun(ctx, "jr_c")
	newest, _ := db.GetScheduledTaskRun(ctx, "jr_e")
	if oldest.Output != "" || newest.Output != "out" {
		t.Errorf("outputs = %q / %q, want the old one blanked and the new one kept", oldest.Output, newest.Output)
	}
}

func TestSyncSpecScheduledTasks_UpsertsAndRemoves(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	n := 0
	newID := func() (string, error) { n++; return fmt.Sprintf("sct_gen%d", n), nil }

	manual := newTestScheduledTask("sct_manual", "web")
	manual.Name = "hand-made"
	_ = db.SaveScheduledTask(ctx, manual)

	jobs := []ScheduledTask{
		{Name: "a", Command: []string{"x"}, Schedule: "0 1 * * *", Enabled: true, Mode: ScheduledTaskModeFresh},
		{Name: "b", Command: []string{"y"}, Schedule: "0 2 * * *", Enabled: true, Mode: ScheduledTaskModeFresh},
	}
	if err := db.SyncSpecScheduledTasks(ctx, "web", jobs, now, newID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	all, _ := db.ListScheduledTasksForService(ctx, "web")
	if len(all) != 3 {
		t.Fatalf("after first sync %d jobs, want 3", len(all))
	}
	var aID string
	for _, j := range all {
		if j.Name == "a" {
			aID = j.ID
		}
	}

	jobs = []ScheduledTask{{Name: "a", Command: []string{"x2"}, Schedule: "0 5 * * *", Enabled: true, Mode: ScheduledTaskModeFresh}}
	if err := db.SyncSpecScheduledTasks(ctx, "web", jobs, now.Add(time.Hour), newID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	all, _ = db.ListScheduledTasksForService(ctx, "web")
	if len(all) != 2 {
		t.Fatalf("after second sync %d jobs, want a and the hand-made one", len(all))
	}
	for _, j := range all {
		if j.Name == "a" && (j.ID != aID || j.Schedule != "0 5 * * *" || j.Command[0] != "x2" || j.Source != ScheduledTaskSourceSpec) {
			t.Errorf("job a = %+v, want same ID, new schedule and command, spec source", j)
		}
		if j.Name == "hand-made" && j.Source == ScheduledTaskSourceSpec {
			t.Error("a hand-made job was adopted by the spec sync")
		}
	}

	clash := []ScheduledTask{{Name: "hand-made", Command: []string{"z"}, Schedule: "* * * * *", Enabled: true}}
	if err := db.SyncSpecScheduledTasks(ctx, "web", clash, now, newID); err == nil {
		t.Error("syncing a spec job over a hand-made one of the same name succeeded, want an error")
	}
}
