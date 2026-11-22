package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPinnedBackupSurvivesPruneAndDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	for i, id := range []string{"p1", "p2", "p3", "p4"} {
		at := time.Date(2026, 8, 14, 0, i, 0, 0, time.UTC).Format(time.RFC3339)
		if err := db.StartBackupHistory(ctx, BackupHistory{ID: id, DatabaseName: "mydb", TargetID: target.ID, ObjectKey: id, StartedAt: at}); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 1, "", "", at); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetBackupPinned(ctx, "p1", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetBackupPinned(ctx, "nope", true); !errors.Is(err, ErrBackupHistoryNotFound) {
		t.Fatalf("pin unknown = %v, want not found", err)
	}
	if err := db.DeleteBackupHistory(ctx, "p1"); !errors.Is(err, ErrBackupPinned) {
		t.Fatalf("delete pinned = %v, want ErrBackupPinned", err)
	}
	// Keep 1 unpinned: p4 stays, p2 and p3 go, pinned p1 is never pruned.
	pruned, err := db.PruneBackupHistory(ctx, "mydb", 1, time.Time{})
	if err != nil || len(pruned) != 2 {
		t.Fatalf("pruned = %v, %v, want 2 rows", pruned, err)
	}
	pins, err := db.PinnedBackups(ctx, []string{"p1", "p4"})
	if err != nil || !pins["p1"] || pins["p4"] {
		t.Fatalf("pins = %v, %v", pins, err)
	}
	if err := db.SetBackupPinned(ctx, "p1", false); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBackupHistory(ctx, "p1"); err != nil {
		t.Fatalf("delete after unpin: %v", err)
	}
}
