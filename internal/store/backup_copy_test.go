package store

import (
	"context"
	"testing"
)

func TestBackupCopyLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	primary := seedBackupTarget(t, db)
	second := newTestBackupTarget()
	second.ID = "bkt_second"
	second.Name = "second"
	if err := db.SaveBackupTarget(ctx, second); err != nil {
		t.Fatalf("SaveBackupTarget: %v", err)
	}
	for _, h := range []BackupHistory{
		{ID: "bkh_old", DatabaseName: "db", TargetID: primary.ID, ObjectKey: "old", StartedAt: "2026-01-01T00:00:00Z"},
		{ID: "bkh_new", DatabaseName: "db", TargetID: primary.ID, ObjectKey: "new", StartedAt: "2026-02-01T00:00:00Z"},
	} {
		if err := db.StartBackupHistory(ctx, h); err != nil {
			t.Fatalf("StartBackupHistory: %v", err)
		}
	}
	if err := db.FinishBackupHistory(ctx, "bkh_old", BackupStatusSucceeded, 10, "", "", "2026-01-01T00:01:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishBackupHistory(ctx, "bkh_new", BackupStatusSucceeded, 20, "", "", "2026-02-01T00:01:00Z"); err != nil {
		t.Fatal(err)
	}

	if err := db.SetBackupCopySettings(ctx, true, "", "2026-01-15T00:00:00Z"); err == nil {
		t.Fatal("enabling without a target must fail")
	}
	if err := db.SetBackupCopySettings(ctx, true, second.ID, "2026-01-15T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetBackupCopySettings(ctx)
	if err := db.EnqueueBackupCopies(ctx, s.TargetID, s.EnabledAt, "2026-02-01T00:02:00Z"); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimBackupCopies(ctx, 5)
	if err != nil || len(claimed) != 1 || claimed[0].BackupHistoryID != "bkh_new" || claimed[0].ObjectKey != "new" {
		t.Fatalf("claimed = %+v, %v (only the backup after enabling should queue)", claimed, err)
	}
	if again, _ := db.ClaimBackupCopies(ctx, 5); len(again) != 0 {
		t.Fatalf("a claimed copy must not be claimed twice: %+v", again)
	}

	if err := db.FinishBackupCopy(ctx, "bkh_new", 0, "boom", "2026-02-01T00:03:00Z"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := db.ListUncopiedBackupCopies(ctx); len(pending) != 1 || pending[0].Error != "boom" {
		t.Fatalf("failed copy should stay uncopied: %+v", pending)
	}
	retry, _ := db.ClaimBackupCopies(ctx, 5)
	if len(retry) != 1 || retry[0].Attempts != 2 {
		t.Fatalf("failed copy should retry: %+v", retry)
	}
	if err := db.FinishBackupCopy(ctx, "bkh_new", 20, "", "2026-02-01T00:04:00Z"); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListBackupCopies(ctx, 10)
	if len(list) != 1 || list[0].Status != BackupCopySucceeded || list[0].CopiedAt == "" {
		t.Fatalf("list = %+v", list)
	}
}
