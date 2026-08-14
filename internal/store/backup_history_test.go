package store

import (
	"context"
	"errors"
	"testing"
)

func seedBackupTarget(t *testing.T, db *DB) BackupTarget {
	t.Helper()
	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(context.Background(), target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	return target
}

func TestStartAndFinishBackupHistory_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	got := mustListOne(t, db, "mydb")
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}

	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusSucceeded, 4096, "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	got = mustListOne(t, db, "mydb")
	if got.Status != BackupStatusSucceeded {
		t.Errorf("status = %q, want %q", got.Status, BackupStatusSucceeded)
	}
	if got.SizeBytes != 4096 {
		t.Errorf("SizeBytes = %d, want 4096", got.SizeBytes)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty on success", got.Error)
	}
	if got.FinishedAt != "2026-08-14T00:01:00Z" {
		t.Errorf("FinishedAt = %q, want the value passed to Finish", got.FinishedAt)
	}
}

func TestFinishBackupHistory_Failed_RecordsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusFailed, 512, "upload: access denied", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	got := mustListOne(t, db, "mydb")
	if got.Status != BackupStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, BackupStatusFailed)
	}
	if got.Error != "upload: access denied" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
}

func TestFinishBackupHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishBackupHistory(ctx, "bkh_missing", BackupStatusSucceeded, 0, "", "2026-08-14T00:01:00Z")
	if !errors.Is(err, ErrBackupHistoryNotFound) {
		t.Fatalf("FinishBackupHistory() error = %v, want ErrBackupHistoryNotFound", err)
	}
}

func TestListBackupHistory_NewestFirst_ScopedToDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	for _, h := range []BackupHistory{
		{ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID, ObjectKey: "k1", StartedAt: "2026-08-14T00:00:00Z"},
		{ID: "bkh_2", DatabaseName: "mydb", TargetID: target.ID, ObjectKey: "k2", StartedAt: "2026-08-14T00:01:00Z"},
		{ID: "bkh_3", DatabaseName: "otherdb", TargetID: target.ID, ObjectKey: "k3", StartedAt: "2026-08-14T00:02:00Z"},
	} {
		if err := db.StartBackupHistory(ctx, h); err != nil {
			t.Fatalf("StartBackupHistory(%s) error = %v", h.ID, err)
		}
	}

	got, err := db.ListBackupHistory(ctx, "mydb")
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "bkh_2" || got[1].ID != "bkh_1" {
		t.Fatalf("ListBackupHistory(mydb) = %+v, want [bkh_2, bkh_1] newest first, otherdb excluded", got)
	}
}

func mustListOne(t *testing.T, db *DB, databaseName string) BackupHistory {
	t.Helper()
	got, err := db.ListBackupHistory(context.Background(), databaseName)
	if err != nil {
		t.Fatalf("ListBackupHistory(%q) error = %v", databaseName, err)
	}
	if len(got) != 1 {
		t.Fatalf("ListBackupHistory(%q) = %+v, want exactly 1 row", databaseName, got)
	}
	return got[0]
}
