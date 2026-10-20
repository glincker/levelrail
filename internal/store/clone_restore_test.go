package store

import (
	"context"
	"errors"
	"testing"
)

func seedCloneRestoreBackup(t *testing.T, db *DB) BackupHistory {
	t.Helper()
	target := seedBackupTarget(t, db)
	h := BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}
	if err := db.StartBackupHistory(context.Background(), h); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), h.ID, BackupStatusSucceeded, 1, "", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
	return h
}

func TestStartAndFinishCloneRestore_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedCloneRestoreBackup(t, db)

	if err := db.StartCloneRestore(ctx, CloneRestore{
		ID: "clr_1", SourceDatabaseName: "mydb", NewDatabaseName: "mydb-staging",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartCloneRestore() error = %v", err)
	}

	got := mustListOneCloneRestore(t, db, "mydb")
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}
	if got.NewDatabaseName != "mydb-staging" {
		t.Fatalf("NewDatabaseName = %q, want %q", got.NewDatabaseName, "mydb-staging")
	}

	if err := db.FinishCloneRestore(ctx, "clr_1", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z"); err != nil {
		t.Fatalf("FinishCloneRestore() error = %v", err)
	}

	got = mustListOneCloneRestore(t, db, "mydb")
	if got.Status != BackupStatusSucceeded {
		t.Errorf("status = %q, want %q", got.Status, BackupStatusSucceeded)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty on success", got.Error)
	}
	if got.FinishedAt != "2026-08-14T01:05:00Z" {
		t.Errorf("FinishedAt = %q, want the value passed to Finish", got.FinishedAt)
	}
}

func TestFinishCloneRestore_Failed_RecordsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedCloneRestoreBackup(t, db)

	if err := db.StartCloneRestore(ctx, CloneRestore{
		ID: "clr_1", SourceDatabaseName: "mydb", NewDatabaseName: "mydb-staging",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartCloneRestore() error = %v", err)
	}

	if err := db.FinishCloneRestore(ctx, "clr_1", BackupStatusFailed, "database never became ready", "2026-08-14T01:05:00Z"); err != nil {
		t.Fatalf("FinishCloneRestore() error = %v", err)
	}

	got := mustListOneCloneRestore(t, db, "mydb")
	if got.Status != BackupStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, BackupStatusFailed)
	}
	if got.Error != "database never became ready" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
}

func TestFinishCloneRestore_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishCloneRestore(ctx, "clr_missing", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z")
	if !errors.Is(err, ErrCloneRestoreNotFound) {
		t.Fatalf("FinishCloneRestore() error = %v, want ErrCloneRestoreNotFound", err)
	}
}

func TestListCloneRestores_NewestFirst_ScopedToSourceDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedCloneRestoreBackup(t, db)

	for _, h := range []CloneRestore{
		{ID: "clr_1", SourceDatabaseName: "mydb", NewDatabaseName: "mydb-a", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z"},
		{ID: "clr_2", SourceDatabaseName: "mydb", NewDatabaseName: "mydb-b", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:01:00Z"},
		{ID: "clr_3", SourceDatabaseName: "otherdb", NewDatabaseName: "otherdb-a", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:02:00Z"},
	} {
		if err := db.StartCloneRestore(ctx, h); err != nil {
			t.Fatalf("StartCloneRestore(%s) error = %v", h.ID, err)
		}
	}

	got, err := db.ListCloneRestores(ctx, "mydb")
	if err != nil {
		t.Fatalf("ListCloneRestores() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "clr_2" || got[1].ID != "clr_1" {
		t.Fatalf("ListCloneRestores(mydb) = %+v, want [clr_2, clr_1] newest first, otherdb excluded", got)
	}
}

func mustListOneCloneRestore(t *testing.T, db *DB, sourceDatabaseName string) CloneRestore {
	t.Helper()
	got, err := db.ListCloneRestores(context.Background(), sourceDatabaseName)
	if err != nil {
		t.Fatalf("ListCloneRestores(%q) error = %v", sourceDatabaseName, err)
	}
	if len(got) != 1 {
		t.Fatalf("ListCloneRestores(%q) = %+v, want exactly 1 row", sourceDatabaseName, got)
	}
	return got[0]
}
