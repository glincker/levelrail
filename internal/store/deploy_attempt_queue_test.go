package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func saveAttempt(t *testing.T, db *DB, id, status string, at time.Time) {
	t.Helper()
	a := DeployAttempt{ID: id, ServiceName: "web", Image: "web:" + id, Source: DeployAttemptSourceManual, Status: status, StartedAt: at}
	if status == DeployAttemptStatusQueued {
		a.QueuedAt = &at
	}
	if err := db.SaveDeployAttempt(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedAttemptLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	saveAttempt(t, db, "dep_b", DeployAttemptStatusQueued, base.Add(2*time.Minute))
	saveAttempt(t, db, "dep_a", DeployAttemptStatusQueued, base.Add(time.Minute))
	saveAttempt(t, db, "dep_r", DeployAttemptStatusRunning, base)

	queued, err := db.ListQueuedDeployAttempts(ctx)
	if err != nil || len(queued) != 2 || queued[0].ID != "dep_a" || queued[1].ID != "dep_b" || queued[0].QueuedAt == nil {
		t.Fatalf("queued = %+v, %v", queued, err)
	}
	if running, _ := db.ListRunningDeployAttempts(ctx); len(running) != 1 || running[0].ID != "dep_r" {
		t.Fatalf("running = %+v", running)
	}

	now := time.Now()
	moved, err := db.StartQueuedDeployAttempt(ctx, "dep_a", now)
	if err != nil || !moved {
		t.Fatalf("start = %v, %v", moved, err)
	}
	if moved, _ := db.StartQueuedDeployAttempt(ctx, "dep_a", now); moved {
		t.Fatal("a second start must be a no-op")
	}
	a, _ := db.GetDeployAttempt(ctx, "dep_a")
	if a.Status != DeployAttemptStatusRunning || !a.StartedAt.After(base.Add(time.Minute)) || a.QueuedAt == nil {
		t.Fatalf("started attempt = %+v", a)
	}
}

func TestCancelDeployAttemptIsConditionalAndSticks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	saveAttempt(t, db, "dep_1", DeployAttemptStatusRunning, time.Now())

	if moved, _ := db.CancelDeployAttempt(ctx, "dep_1", DeployAttemptStatusQueued, "alice", time.Now()); moved {
		t.Fatal("cancel from the wrong status must not move")
	}
	moved, err := db.CancelDeployAttempt(ctx, "dep_1", DeployAttemptStatusRunning, "alice", time.Now())
	if err != nil || !moved {
		t.Fatalf("cancel = %v, %v", moved, err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_1", DeployAttemptStatusFailed, time.Now(), "context canceled"); err != nil {
		t.Fatalf("a late finish must be a no-op, got %v", err)
	}
	a, _ := db.GetDeployAttempt(ctx, "dep_1")
	if a.Status != DeployAttemptStatusCanceled || a.CanceledBy != "alice" || a.Error != "" || a.FinishedAt == nil {
		t.Fatalf("attempt = %+v", a)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_missing", DeployAttemptStatusFailed, time.Now(), ""); !errors.Is(err, ErrDeployAttemptNotFound) {
		t.Fatalf("finish of a missing attempt = %v", err)
	}
}

func TestSupersedeQueuedOnlyTouchesQueued(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	saveAttempt(t, db, "dep_q", DeployAttemptStatusQueued, time.Now())
	saveAttempt(t, db, "dep_r", DeployAttemptStatusRunning, time.Now())

	if moved, _ := db.SupersedeQueuedDeployAttempt(ctx, "dep_r", "dep_new", time.Now()); moved {
		t.Fatal("a running attempt must never be superseded")
	}
	if moved, err := db.SupersedeQueuedDeployAttempt(ctx, "dep_q", "dep_new", time.Now()); err != nil || !moved {
		t.Fatalf("supersede = %v, %v", moved, err)
	}
	a, _ := db.GetDeployAttempt(ctx, "dep_q")
	if a.Status != DeployAttemptStatusSuperseded || a.SupersededBy != "dep_new" || a.Reason != DeployReasonSuperseded {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestFinishRunningDeployAttemptDoesNotOverwrite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	saveAttempt(t, db, "dep_1", DeployAttemptStatusRunning, time.Now())
	if moved, _ := db.FinishRunningDeployAttempt(ctx, "dep_1", DeployAttemptStatusSucceeded, time.Now(), ""); !moved {
		t.Fatal("first finish must move")
	}
	if moved, _ := db.FinishRunningDeployAttempt(ctx, "dep_1", DeployAttemptStatusFailed, time.Now(), "boom"); moved {
		t.Fatal("finishing an already finished attempt must be a no-op")
	}
	if a, _ := db.GetDeployAttempt(ctx, "dep_1"); a.Status != DeployAttemptStatusSucceeded || a.Error != "" {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestServiceCancelSuperseded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.GetServiceCancelSuperseded(ctx, "ghost"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing app = %v", err)
	}
	if err := db.SetServiceCancelSuperseded(ctx, "ghost", true); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing app set = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if on, err := db.GetServiceCancelSuperseded(ctx, "web"); err != nil || on {
		t.Fatalf("default = %v, %v", on, err)
	}
	if err := db.SetServiceCancelSuperseded(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:2", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if on, _ := db.GetServiceCancelSuperseded(ctx, "web"); !on {
		t.Fatal("an ordinary app save must not reset the setting")
	}
}
