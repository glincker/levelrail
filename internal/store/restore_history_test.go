package store

import (
	"context"
	"testing"
)

func TestStartRestoreHistory_Volume(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	// restore_history.backup_history_id is a real foreign key
	// (migrations/0019), so a backup_history row must exist first.
	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	if err := db.StartRestoreHistory(ctx, RestoreHistory{
		ID: "rsh_1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		BackupHistoryID: "bkh_1", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartRestoreHistory() error = %v", err)
	}

	got, err := db.ListServiceVolumeRestoreHistory(ctx, "web", "data")
	if err != nil {
		t.Fatalf("ListServiceVolumeRestoreHistory() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListServiceVolumeRestoreHistory() = %+v, want exactly 1 row", got)
	}
	if got[0].Status != BackupStatusRunning || got[0].DatabaseName != "" {
		t.Errorf("row = %+v, want status=running, empty DatabaseName", got[0])
	}

	if err := db.FinishRestoreHistory(ctx, "rsh_1", BackupStatusSucceeded, "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishRestoreHistory() error = %v", err)
	}

	got, err = db.ListServiceVolumeRestoreHistory(ctx, "web", "data")
	if err != nil {
		t.Fatalf("ListServiceVolumeRestoreHistory() after finish error = %v", err)
	}
	if got[0].Status != BackupStatusSucceeded {
		t.Errorf("status = %q, want %q", got[0].Status, BackupStatusSucceeded)
	}

	// A plain database-scoped list must never pick up this volume row.
	dbRows, err := db.ListRestoreHistory(ctx, "web")
	if err != nil {
		t.Fatalf("ListRestoreHistory() error = %v", err)
	}
	if len(dbRows) != 0 {
		t.Errorf("ListRestoreHistory(%q) = %+v, want 0 rows", "web", dbRows)
	}
}
