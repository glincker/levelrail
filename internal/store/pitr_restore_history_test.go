package store

import (
	"context"
	"errors"
	"testing"
)

func TestStartAndFinishPITRRestoreHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	seedSucceeded(t, db, target.ID, "bbh_1", "2026-08-14T00:00:00Z")

	if err := db.StartPITRRestoreHistory(ctx, PITRRestoreHistory{
		ID: "pitr_1", DatabaseName: "mydb", BaseBackupHistoryID: "bbh_1",
		TargetTimestamp: "2026-08-14T00:30:00Z", StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartPITRRestoreHistory() error = %v", err)
	}

	got, err := db.GetPITRRestoreHistory(ctx, "pitr_1")
	if err != nil {
		t.Fatalf("GetPITRRestoreHistory() error = %v", err)
	}
	if got.Status != BackupStatusRunning || got.TargetTimestamp != "2026-08-14T00:30:00Z" {
		t.Fatalf("got = %+v, want running/2026-08-14T00:30:00Z", got)
	}

	if err := db.FinishPITRRestoreHistory(ctx, "pitr_1", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z"); err != nil {
		t.Fatalf("FinishPITRRestoreHistory() error = %v", err)
	}
	got, err = db.GetPITRRestoreHistory(ctx, "pitr_1")
	if err != nil {
		t.Fatalf("GetPITRRestoreHistory() error = %v", err)
	}
	if got.Status != BackupStatusSucceeded || got.FinishedAt != "2026-08-14T01:05:00Z" {
		t.Fatalf("got = %+v, want succeeded/2026-08-14T01:05:00Z", got)
	}
}

func TestFinishPITRRestoreHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.FinishPITRRestoreHistory(context.Background(), "pitr_missing", BackupStatusFailed, "boom", "2026-08-14T00:00:00Z")
	if !errors.Is(err, ErrPITRRestoreHistoryNotFound) {
		t.Fatalf("error = %v, want ErrPITRRestoreHistoryNotFound", err)
	}
}

func TestGetPITRRestoreHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetPITRRestoreHistory(context.Background(), "pitr_missing")
	if !errors.Is(err, ErrPITRRestoreHistoryNotFound) {
		t.Fatalf("error = %v, want ErrPITRRestoreHistoryNotFound", err)
	}
}

func TestListPITRRestoreHistory_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	seedSucceeded(t, db, target.ID, "bbh_1", "2026-08-14T00:00:00Z")

	seed := []struct{ id, started string }{
		{"pitr_1", "2026-08-14T01:00:00Z"},
		{"pitr_2", "2026-08-14T02:00:00Z"},
		{"pitr_3", "2026-08-14T00:30:00Z"},
	}
	for _, s := range seed {
		if err := db.StartPITRRestoreHistory(ctx, PITRRestoreHistory{
			ID: s.id, DatabaseName: "mydb", BaseBackupHistoryID: "bbh_1",
			TargetTimestamp: s.started, StartedAt: s.started,
		}); err != nil {
			t.Fatalf("StartPITRRestoreHistory(%s) error = %v", s.id, err)
		}
	}

	got, err := db.ListPITRRestoreHistory(ctx, "mydb")
	if err != nil {
		t.Fatalf("ListPITRRestoreHistory() error = %v", err)
	}
	want := []string{"pitr_2", "pitr_1", "pitr_3"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}
