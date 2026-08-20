package store

import (
	"context"
	"errors"
	"testing"
)

func seedScheduledTaskApp(t *testing.T, db *DB) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
}

func newTestScheduledTask(id string) ScheduledTask {
	return ScheduledTask{
		ID: id, ServiceName: "web", Name: "cron", Command: "php artisan schedule:run",
		Schedule: "* * * * *", Enabled: true, TimeoutSeconds: 60,
		CreatedAt: "2026-08-14T00:00:00Z", UpdatedAt: "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	want := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, want); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	got, err := db.GetScheduledTask(ctx, "sct_1")
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if got != want {
		t.Errorf("GetScheduledTask() = %+v, want %+v", got, want)
	}
}

func TestGetScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetScheduledTask(context.Background(), "sct_missing")
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestListScheduledTasksForService_ScopedToService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "levelrail/other:1", Port: 4000}); err != nil {
		t.Fatalf("seed other app: %v", err)
	}

	webTask := newTestScheduledTask("sct_1")
	otherTask := newTestScheduledTask("sct_2")
	otherTask.ServiceName = "other"
	if err := db.SaveScheduledTask(ctx, webTask); err != nil {
		t.Fatalf("SaveScheduledTask(web) error = %v", err)
	}
	if err := db.SaveScheduledTask(ctx, otherTask); err != nil {
		t.Fatalf("SaveScheduledTask(other) error = %v", err)
	}

	got, err := db.ListScheduledTasksForService(ctx, "web")
	if err != nil {
		t.Fatalf("ListScheduledTasksForService() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "sct_1" {
		t.Fatalf("ListScheduledTasksForService(web) = %+v, want exactly [sct_1]", got)
	}
}

func TestListEnabledScheduledTasks_ExcludesDisabled(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	enabled := newTestScheduledTask("sct_1")
	disabled := newTestScheduledTask("sct_2")
	disabled.Enabled = false
	if err := db.SaveScheduledTask(ctx, enabled); err != nil {
		t.Fatalf("SaveScheduledTask(enabled) error = %v", err)
	}
	if err := db.SaveScheduledTask(ctx, disabled); err != nil {
		t.Fatalf("SaveScheduledTask(disabled) error = %v", err)
	}

	got, err := db.ListEnabledScheduledTasks(ctx)
	if err != nil {
		t.Fatalf("ListEnabledScheduledTasks() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "sct_1" {
		t.Fatalf("ListEnabledScheduledTasks() = %+v, want exactly [sct_1]", got)
	}
}

func TestUpdateScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	task.Name = "renamed"
	task.Command = "true"
	task.Schedule = "*/5 * * * *"
	task.Enabled = false
	task.TimeoutSeconds = 120
	task.UpdatedAt = "2026-08-14T01:00:00Z"
	if err := db.UpdateScheduledTask(ctx, task); err != nil {
		t.Fatalf("UpdateScheduledTask() error = %v", err)
	}

	got, err := db.GetScheduledTask(ctx, "sct_1")
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if got.Name != "renamed" || got.Command != "true" || got.Schedule != "*/5 * * * *" || got.Enabled || got.TimeoutSeconds != 120 {
		t.Errorf("GetScheduledTask() after update = %+v, want the updated fields", got)
	}
	// ServiceName and CreatedAt must never change via update.
	if got.ServiceName != "web" || got.CreatedAt != "2026-08-14T00:00:00Z" {
		t.Errorf("GetScheduledTask() after update = %+v, want ServiceName/CreatedAt untouched", got)
	}
}

func TestUpdateScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateScheduledTask(context.Background(), newTestScheduledTask("sct_missing"))
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("UpdateScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestDeleteScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}
	if err := db.DeleteScheduledTask(ctx, "sct_1"); err != nil {
		t.Fatalf("DeleteScheduledTask() error = %v", err)
	}
	if _, err := db.GetScheduledTask(ctx, "sct_1"); !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() after delete error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestDeleteScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeleteScheduledTask(context.Background(), "sct_missing")
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("DeleteScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

// TestDeleteScheduledTask_CascadesRuns proves migrations/0048's ON
// DELETE CASCADE actually fires: deleting a task must also remove every
// scheduled_task_runs row referencing it.
func TestDeleteScheduledTask_CascadesRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}
	if err := db.StartScheduledTaskRun(ctx, ScheduledTaskRun{ID: "sctr_1", ScheduledTaskID: "sct_1", StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("StartScheduledTaskRun() error = %v", err)
	}

	if err := db.DeleteScheduledTask(ctx, "sct_1"); err != nil {
		t.Fatalf("DeleteScheduledTask() error = %v", err)
	}

	runs, err := db.ListScheduledTaskRuns(ctx, "sct_1")
	if err != nil {
		t.Fatalf("ListScheduledTaskRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("ListScheduledTaskRuns() after task delete = %+v, want 0 (cascade)", runs)
	}
}

// TestDeleteDesiredService_CascadesScheduledTasks proves the other
// cascade direction: deleting the owning service removes its scheduled
// tasks.
func TestDeleteDesiredService_CascadesScheduledTasks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	if _, err := db.GetScheduledTask(ctx, "sct_1"); !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() after owning service delete error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestStartAndFinishScheduledTaskRun(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)
	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	if err := db.StartScheduledTaskRun(ctx, ScheduledTaskRun{ID: "sctr_1", ScheduledTaskID: "sct_1", StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("StartScheduledTaskRun() error = %v", err)
	}

	runs, err := db.ListScheduledTaskRuns(ctx, "sct_1")
	if err != nil {
		t.Fatalf("ListScheduledTaskRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Status != ScheduledTaskRunStatusRunning {
		t.Fatalf("ListScheduledTaskRuns() after start = %+v, want one running row", runs)
	}

	if err := db.FinishScheduledTaskRun(ctx, "sctr_1", ScheduledTaskRunStatusSucceeded, 0, "done\n", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishScheduledTaskRun() error = %v", err)
	}

	runs, err = db.ListScheduledTaskRuns(ctx, "sct_1")
	if err != nil {
		t.Fatalf("ListScheduledTaskRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListScheduledTaskRuns() = %+v, want 1", runs)
	}
	got := runs[0]
	if got.Status != ScheduledTaskRunStatusSucceeded || got.Output != "done\n" || got.FinishedAt != "2026-08-14T00:01:00Z" {
		t.Errorf("ListScheduledTaskRuns()[0] = %+v, want succeeded/done/finished timestamp set", got)
	}
}

func TestFinishScheduledTaskRun_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.FinishScheduledTaskRun(context.Background(), "sctr_missing", ScheduledTaskRunStatusFailed, 1, "", "boom", "2026-08-14T00:01:00Z")
	if !errors.Is(err, ErrScheduledTaskRunNotFound) {
		t.Fatalf("FinishScheduledTaskRun() error = %v, want ErrScheduledTaskRunNotFound", err)
	}
}

func TestListScheduledTaskRuns_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)
	task := newTestScheduledTask("sct_1")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	for _, r := range []ScheduledTaskRun{
		{ID: "sctr_1", ScheduledTaskID: "sct_1", StartedAt: "2026-08-14T00:00:00Z"},
		{ID: "sctr_2", ScheduledTaskID: "sct_1", StartedAt: "2026-08-14T00:01:00Z"},
	} {
		if err := db.StartScheduledTaskRun(ctx, r); err != nil {
			t.Fatalf("StartScheduledTaskRun(%s) error = %v", r.ID, err)
		}
	}

	got, err := db.ListScheduledTaskRuns(ctx, "sct_1")
	if err != nil {
		t.Fatalf("ListScheduledTaskRuns() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "sctr_2" || got[1].ID != "sctr_1" {
		t.Fatalf("ListScheduledTaskRuns() = %+v, want [sctr_2, sctr_1] newest first", got)
	}
}
