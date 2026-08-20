package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedScheduledTaskService(t *testing.T, db *DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed service %q: %v", name, err)
	}
}

func newTestScheduledTask(id, service string) ScheduledTask {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	return ScheduledTask{
		ID:          id,
		ServiceName: service,
		Command:     []string{"sh", "-c", "echo hi"},
		Schedule:    "0 3 * * *",
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestSaveAndGetScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	want := newTestScheduledTask("st_1", "web")
	if err := db.SaveScheduledTask(ctx, want); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	got, err := db.GetScheduledTask(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if got.ID != want.ID || got.ServiceName != want.ServiceName || got.Schedule != want.Schedule || got.Enabled != want.Enabled {
		t.Errorf("GetScheduledTask() = %+v, want %+v", got, want)
	}
	if len(got.Command) != len(want.Command) {
		t.Fatalf("GetScheduledTask() Command = %v, want %v", got.Command, want.Command)
	}
	for i := range want.Command {
		if got.Command[i] != want.Command[i] {
			t.Errorf("GetScheduledTask() Command = %v, want %v", got.Command, want.Command)
		}
	}
	if got.LastRunAt != nil {
		t.Errorf("GetScheduledTask() LastRunAt = %v, want nil (never run yet)", got.LastRunAt)
	}
	if got.LastRunStatus != "" || got.LastRunOutput != "" {
		t.Errorf("GetScheduledTask() last run fields not empty: status=%q output=%q", got.LastRunStatus, got.LastRunOutput)
	}
}

func TestGetScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetScheduledTask(ctx, "st_missing")
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestListScheduledTasksForService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	seedScheduledTaskService(t, db, "worker")

	a := newTestScheduledTask("st_a", "web")
	a.CreatedAt = time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	b := newTestScheduledTask("st_b", "web")
	b.CreatedAt = time.Date(2026, 8, 14, 0, 0, 1, 0, time.UTC)
	other := newTestScheduledTask("st_c", "worker")

	for _, task := range []ScheduledTask{b, a, other} {
		if err := db.SaveScheduledTask(ctx, task); err != nil {
			t.Fatalf("SaveScheduledTask(%q) error = %v", task.ID, err)
		}
	}

	got, err := db.ListScheduledTasksForService(ctx, "web")
	if err != nil {
		t.Fatalf("ListScheduledTasksForService() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "st_a" || got[1].ID != "st_b" {
		t.Fatalf("ListScheduledTasksForService() = %+v, want [st_a, st_b] in creation order", got)
	}
}

func TestListEnabledScheduledTasks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	enabled := newTestScheduledTask("st_enabled", "web")
	disabled := newTestScheduledTask("st_disabled", "web")
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
	if len(got) != 1 || got[0].ID != "st_enabled" {
		t.Fatalf("ListEnabledScheduledTasks() = %+v, want only st_enabled", got)
	}
}

func TestUpdateScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	task := newTestScheduledTask("st_1", "web")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	newCommand := []string{"sh", "-c", "echo bye"}
	updatedAt := task.UpdatedAt.Add(time.Hour)
	if err := db.UpdateScheduledTask(ctx, task.ID, newCommand, "*/5 * * * *", false, updatedAt); err != nil {
		t.Fatalf("UpdateScheduledTask() error = %v", err)
	}

	got, err := db.GetScheduledTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if got.Schedule != "*/5 * * * *" || got.Enabled {
		t.Errorf("GetScheduledTask() after update = %+v, want schedule=*/5 * * * * enabled=false", got)
	}
	if len(got.Command) != 3 || got.Command[2] != "echo bye" {
		t.Errorf("GetScheduledTask() Command after update = %v, want %v", got.Command, newCommand)
	}
}

func TestUpdateScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateScheduledTask(ctx, "st_missing", []string{"echo"}, "* * * * *", true, time.Now())
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("UpdateScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestRecordScheduledTaskRun(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	task := newTestScheduledTask("st_1", "web")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	ranAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	if err := db.RecordScheduledTaskRun(ctx, task.ID, ranAt, ScheduledTaskStatusSuccess, "hi\n"); err != nil {
		t.Fatalf("RecordScheduledTaskRun() error = %v", err)
	}

	got, err := db.GetScheduledTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask() error = %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(ranAt) {
		t.Errorf("GetScheduledTask() LastRunAt = %v, want %v", got.LastRunAt, ranAt)
	}
	if got.LastRunStatus != ScheduledTaskStatusSuccess {
		t.Errorf("GetScheduledTask() LastRunStatus = %q, want %q", got.LastRunStatus, ScheduledTaskStatusSuccess)
	}
	if got.LastRunOutput != "hi\n" {
		t.Errorf("GetScheduledTask() LastRunOutput = %q, want %q", got.LastRunOutput, "hi\n")
	}
}

func TestRecordScheduledTaskRun_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.RecordScheduledTaskRun(ctx, "st_missing", time.Now(), ScheduledTaskStatusFailed, "")
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("RecordScheduledTaskRun() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestDeleteScheduledTask(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	task := newTestScheduledTask("st_1", "web")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}
	if err := db.DeleteScheduledTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteScheduledTask() error = %v", err)
	}

	_, err := db.GetScheduledTask(ctx, task.ID)
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() after delete error = %v, want ErrScheduledTaskNotFound", err)
	}
}

func TestDeleteScheduledTask_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteScheduledTask(ctx, "st_missing")
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("DeleteScheduledTask() error = %v, want ErrScheduledTaskNotFound", err)
	}
}

// TestDeleteDesiredService_CascadesScheduledTasks proves
// migrations/0048_scheduled_tasks.sql's ON DELETE CASCADE actually
// removes a service's scheduled tasks when the service itself is
// deleted, the same "no orphaned child rows" guarantee service_domains
// already gets from its own identical FK shape.
func TestDeleteDesiredService_CascadesScheduledTasks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")

	task := newTestScheduledTask("st_1", "web")
	if err := db.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask() error = %v", err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	_, err := db.GetScheduledTask(ctx, task.ID)
	if !errors.Is(err, ErrScheduledTaskNotFound) {
		t.Fatalf("GetScheduledTask() after service delete error = %v, want ErrScheduledTaskNotFound", err)
	}
}
