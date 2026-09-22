package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartAndFinishBaseBackupHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBaseBackupHistory(ctx, BaseBackupHistory{
		ID: "bbh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/base-1.tar",
		StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBaseBackupHistory() error = %v", err)
	}

	got, err := db.GetBaseBackupHistory(ctx, "bbh_1")
	if err != nil {
		t.Fatalf("GetBaseBackupHistory() error = %v", err)
	}
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}

	if err := db.FinishBaseBackupHistory(ctx, "bbh_1", BackupStatusSucceeded, 31651840, "0/3000028", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBaseBackupHistory() error = %v", err)
	}

	got, err = db.GetBaseBackupHistory(ctx, "bbh_1")
	if err != nil {
		t.Fatalf("GetBaseBackupHistory() error = %v", err)
	}
	if got.Status != BackupStatusSucceeded || got.SizeBytes != 31651840 || got.LSN != "0/3000028" {
		t.Fatalf("got = %+v, want succeeded/31651840/0-3000028", got)
	}
}

func TestFinishBaseBackupHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.FinishBaseBackupHistory(context.Background(), "bbh_missing", BackupStatusFailed, 0, "", "boom", "2026-08-14T00:00:00Z")
	if !errors.Is(err, ErrBaseBackupHistoryNotFound) {
		t.Fatalf("error = %v, want ErrBaseBackupHistoryNotFound", err)
	}
}

func TestGetBaseBackupHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetBaseBackupHistory(context.Background(), "bbh_missing")
	if !errors.Is(err, ErrBaseBackupHistoryNotFound) {
		t.Fatalf("error = %v, want ErrBaseBackupHistoryNotFound", err)
	}
}

func TestListBaseBackupHistory_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	seed := []struct{ id, started string }{
		{"bbh_1", "2026-08-14T00:00:00Z"},
		{"bbh_2", "2026-08-15T00:00:00Z"},
		{"bbh_3", "2026-08-13T00:00:00Z"},
	}
	for _, s := range seed {
		if err := db.StartBaseBackupHistory(ctx, BaseBackupHistory{
			ID: s.id, DatabaseName: "mydb", TargetID: target.ID,
			ObjectKey: s.id + ".tar", StartedAt: s.started,
		}); err != nil {
			t.Fatalf("StartBaseBackupHistory(%s) error = %v", s.id, err)
		}
		if err := db.FinishBaseBackupHistory(ctx, s.id, BackupStatusSucceeded, 100, "0/1", "", s.started); err != nil {
			t.Fatalf("FinishBaseBackupHistory(%s) error = %v", s.id, err)
		}
	}

	got, err := db.ListBaseBackupHistory(ctx, "mydb")
	if err != nil {
		t.Fatalf("ListBaseBackupHistory() error = %v", err)
	}
	want := []string{"bbh_2", "bbh_1", "bbh_3"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestGetOldestSucceededBaseBackup(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if _, ok, err := db.GetOldestSucceededBaseBackup(ctx, "mydb"); err != nil || ok {
		t.Fatalf("GetOldestSucceededBaseBackup() on empty history = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	// A running attempt (never finished) must not count as the oldest
	// succeeded backup: it has no complete, restorable object yet.
	if err := db.StartBaseBackupHistory(ctx, BaseBackupHistory{
		ID: "bbh_running", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "r.tar", StartedAt: "2026-08-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBaseBackupHistory(running) error = %v", err)
	}

	seedSucceeded(t, db, target.ID, "bbh_older", "2026-08-12T00:00:00Z")
	seedSucceeded(t, db, target.ID, "bbh_newer", "2026-08-16T00:00:00Z")

	got, ok, err := db.GetOldestSucceededBaseBackup(ctx, "mydb")
	if err != nil || !ok {
		t.Fatalf("GetOldestSucceededBaseBackup() = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if got.ID != "bbh_older" {
		t.Fatalf("got.ID = %q, want %q", got.ID, "bbh_older")
	}
}

func TestGetLatestBaseBackupBefore(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	seedSucceeded(t, db, target.ID, "bbh_1", "2026-08-10T00:00:00Z")
	seedSucceeded(t, db, target.ID, "bbh_2", "2026-08-12T00:00:00Z")
	seedSucceeded(t, db, target.ID, "bbh_3", "2026-08-14T00:00:00Z")

	cutoff := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	got, ok, err := db.GetLatestBaseBackupBefore(ctx, "mydb", cutoff)
	if err != nil || !ok {
		t.Fatalf("GetLatestBaseBackupBefore() = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if got.ID != "bbh_2" {
		t.Fatalf("got.ID = %q, want %q (the newest succeeded backup still <= cutoff)", got.ID, "bbh_2")
	}

	tooEarly := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if _, ok, err := db.GetLatestBaseBackupBefore(ctx, "mydb", tooEarly); err != nil || ok {
		t.Fatalf("GetLatestBaseBackupBefore(too early) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

func seedSucceeded(t *testing.T, db *DB, targetID, id, startedAt string) {
	t.Helper()
	ctx := context.Background()
	if err := db.StartBaseBackupHistory(ctx, BaseBackupHistory{
		ID: id, DatabaseName: "mydb", TargetID: targetID,
		ObjectKey: id + ".tar", StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("StartBaseBackupHistory(%s) error = %v", id, err)
	}
	if err := db.FinishBaseBackupHistory(ctx, id, BackupStatusSucceeded, 100, "0/1", "", startedAt); err != nil {
		t.Fatalf("FinishBaseBackupHistory(%s) error = %v", id, err)
	}
}
