package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComposeServiceBackupRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	c := ComposeServiceBackup{App: "shop", Service: "db", Engine: EnginePostgres, BackupTargetID: target.ID, BackupSchedule: "0 3 * * *", BackupRetain: 3, Enabled: true}
	if err := db.UpsertComposeServiceBackup(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetComposeServiceBackup(ctx, "shop", "db")
	if err != nil || got.BackupSchedule != "0 3 * * *" || got.BackupTargetID != target.ID || !got.Enabled || got.BackupRetain != 3 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if n, _ := db.CountScheduledComposeServiceBackups(ctx); n != 1 {
		t.Fatalf("scheduled count = %d, want 1", n)
	}

	c.Enabled = false
	if err := db.UpsertComposeServiceBackup(ctx, c); err != nil {
		t.Fatalf("upsert disabled: %v", err)
	}
	if sched, _ := db.ListScheduledComposeServices(ctx); len(sched) != 0 {
		t.Fatalf("disabled row listed as scheduled: %+v", sched)
	}
	if all, _ := db.ListComposeServiceBackups(ctx, "shop"); len(all) != 1 {
		t.Fatalf("list by app = %d rows, want 1", len(all))
	}

	if err := db.DeleteComposeServiceBackup(ctx, "shop", "db"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.GetComposeServiceBackup(ctx, "shop", "db"); !errors.Is(err, ErrComposeServiceBackupNotFound) {
		t.Fatalf("get after delete err = %v, want not found", err)
	}
	if err := db.DeleteComposeServiceBackup(ctx, "shop", "db"); !errors.Is(err, ErrComposeServiceBackupNotFound) {
		t.Fatalf("second delete err = %v, want not found", err)
	}
}

func TestComposeServiceBackupRunsAndPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"csr_1", "csr_2", "csr_3"} {
		started := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := db.StartComposeServiceBackupRun(ctx, ComposeServiceBackupRun{ID: id, App: "shop", Service: "db", TargetID: "bkt_x", ObjectKey: "k/" + id, StartedAt: started}); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		status, msg := BackupStatusSucceeded, ""
		if id == "csr_3" {
			status, msg = BackupStatusFailed, "boom"
		}
		if err := db.FinishComposeServiceBackupRun(ctx, id, status, 10, "sum", msg, started); err != nil {
			t.Fatalf("finish %s: %v", id, err)
		}
	}
	runs, err := db.ListComposeServiceBackupRuns(ctx, "shop", "db", 10)
	if err != nil || len(runs) != 3 || runs[0].ID != "csr_3" || runs[0].Error != "boom" {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	// Keep 1 succeeded: csr_2 stays, csr_1 goes, the failed csr_3 is untouched.
	pruned, err := db.PruneComposeServiceBackupRuns(ctx, "shop", "db", 1, time.Time{})
	if err != nil || len(pruned) != 1 || pruned[0].ObjectKey != "k/csr_1" || pruned[0].TargetID != "bkt_x" {
		t.Fatalf("pruned = %+v, %v", pruned, err)
	}
	if runs, _ = db.ListComposeServiceBackupRuns(ctx, "shop", "db", 10); len(runs) != 2 {
		t.Fatalf("runs after prune = %d, want 2", len(runs))
	}
}

func TestDeleteComposeServiceBackupsForAppKeepsRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, svc := range []string{"db", "cache"} {
		c := ComposeServiceBackup{App: "shop", Service: svc, Engine: EnginePostgres, BackupSchedule: "0 3 * * *", Enabled: true}
		if err := db.UpsertComposeServiceBackup(ctx, c); err != nil {
			t.Fatalf("upsert %s: %v", svc, err)
		}
	}
	if err := db.StartComposeServiceBackupRun(ctx, ComposeServiceBackupRun{ID: "csr_keep", App: "shop", Service: "db", ObjectKey: "k/1", StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := db.DeleteComposeServiceBackupsForApp(ctx, "shop"); err != nil {
		t.Fatalf("delete for app: %v", err)
	}
	if rows, _ := db.ListComposeServiceBackups(ctx, "shop"); len(rows) != 0 {
		t.Fatalf("schedules left = %d, want 0", len(rows))
	}
	if runs, _ := db.ListComposeServiceBackupRuns(ctx, "shop", "db", 10); len(runs) != 1 {
		t.Fatalf("runs = %d, want history kept", len(runs))
	}
	if ok, err := db.ComposeAppExists(ctx, "shop"); err != nil || ok {
		t.Fatalf("ComposeAppExists = %v, %v, want false", ok, err)
	}
}

func TestComposeAppExistsSeesAppRowAndMemberServices(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	app := newTestApp()
	seedAppWithWebWorker(t, db, app)
	if ok, err := db.ComposeAppExists(ctx, app.ID); err != nil || !ok {
		t.Fatalf("ComposeAppExists for a live app = %v, %v, want true", ok, err)
	}
	if ok, err := db.ComposeAppExists(ctx, "app_gone"); err != nil || ok {
		t.Fatalf("ComposeAppExists unknown = %v, %v, want false", ok, err)
	}
}
