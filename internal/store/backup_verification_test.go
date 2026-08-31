package store

import (
	"context"
	"errors"
	"testing"
)

func seedSucceededBackupHistory(t *testing.T, db *DB) BackupHistory {
	t.Helper()
	ctx := context.Background()
	const id = "bkh_1"
	target := seedBackupTarget(t, db)
	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: id, DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 4096, "abc123checksum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
	h, err := db.GetBackupHistory(ctx, id)
	if err != nil {
		t.Fatalf("GetBackupHistory() error = %v", err)
	}
	return h
}

func TestStartAndFinishBackupVerification_Passed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	h := seedSucceededBackupHistory(t, db)

	if err := db.StartBackupVerification(ctx, BackupVerification{
		ID: "bkv_1", BackupHistoryID: h.ID, CheckedBy: "alice", StartedAt: "2026-08-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupVerification() error = %v", err)
	}

	got, err := db.GetLatestBackupVerification(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetLatestBackupVerification() error = %v", err)
	}
	if got.Status != BackupVerificationStatusRunning {
		t.Errorf("status after Start = %q, want %q", got.Status, BackupVerificationStatusRunning)
	}
	if got.CheckedBy != "alice" {
		t.Errorf("CheckedBy = %q, want %q", got.CheckedBy, "alice")
	}

	if err := db.FinishBackupVerification(ctx, "bkv_1", BackupVerificationStatusPassed, true, true, true, 4096, "", "2026-08-15T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupVerification() error = %v", err)
	}

	got, err = db.GetLatestBackupVerification(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetLatestBackupVerification() error = %v", err)
	}
	if got.Status != BackupVerificationStatusPassed {
		t.Errorf("status = %q, want %q", got.Status, BackupVerificationStatusPassed)
	}
	if !got.ChecksumMatch || !got.SizeMatch || !got.FormatValid {
		t.Errorf("checks = %+v, want all true", got)
	}
	if got.DownloadedBytes != 4096 {
		t.Errorf("DownloadedBytes = %d, want 4096", got.DownloadedBytes)
	}
	if got.FinishedAt != "2026-08-15T00:01:00Z" {
		t.Errorf("FinishedAt = %q, want the value passed to Finish", got.FinishedAt)
	}
}

func TestFinishBackupVerification_Failed_RecordsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	h := seedSucceededBackupHistory(t, db)

	if err := db.StartBackupVerification(ctx, BackupVerification{
		ID: "bkv_1", BackupHistoryID: h.ID, StartedAt: "2026-08-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupVerification() error = %v", err)
	}
	if err := db.FinishBackupVerification(ctx, "bkv_1", BackupVerificationStatusFailed, false, true, true, 4096, "checksum mismatch", "2026-08-15T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupVerification() error = %v", err)
	}

	got, err := db.GetLatestBackupVerification(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetLatestBackupVerification() error = %v", err)
	}
	if got.Status != BackupVerificationStatusFailed {
		t.Errorf("status = %q, want %q", got.Status, BackupVerificationStatusFailed)
	}
	if got.ChecksumMatch {
		t.Error("ChecksumMatch = true, want false")
	}
	if got.Error != "checksum mismatch" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
}

func TestFinishBackupVerification_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishBackupVerification(ctx, "bkv_missing", BackupVerificationStatusPassed, true, true, true, 0, "", "2026-08-15T00:01:00Z")
	if !errors.Is(err, ErrBackupVerificationNotFound) {
		t.Fatalf("FinishBackupVerification() error = %v, want ErrBackupVerificationNotFound", err)
	}
}

func TestGetLatestBackupVerification_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	h := seedSucceededBackupHistory(t, db)

	_, err := db.GetLatestBackupVerification(ctx, h.ID)
	if !errors.Is(err, ErrBackupVerificationNotFound) {
		t.Fatalf("GetLatestBackupVerification() error = %v, want ErrBackupVerificationNotFound", err)
	}
}

func TestListBackupVerifications_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	h := seedSucceededBackupHistory(t, db)

	attempts := []struct {
		id, startedAt, finishedAt, status string
	}{
		{"bkv_1", "2026-08-15T00:00:00Z", "2026-08-15T00:01:00Z", BackupVerificationStatusPassed},
		{"bkv_2", "2026-08-16T00:00:00Z", "2026-08-16T00:01:00Z", BackupVerificationStatusFailed},
		{"bkv_3", "2026-08-17T00:00:00Z", "2026-08-17T00:01:00Z", BackupVerificationStatusPassed},
	}
	for _, a := range attempts {
		if err := db.StartBackupVerification(ctx, BackupVerification{ID: a.id, BackupHistoryID: h.ID, StartedAt: a.startedAt}); err != nil {
			t.Fatalf("StartBackupVerification(%s) error = %v", a.id, err)
		}
		if err := db.FinishBackupVerification(ctx, a.id, a.status, true, true, true, 4096, "", a.finishedAt); err != nil {
			t.Fatalf("FinishBackupVerification(%s) error = %v", a.id, err)
		}
	}

	got, err := db.ListBackupVerifications(ctx, h.ID, 50)
	if err != nil {
		t.Fatalf("ListBackupVerifications() error = %v", err)
	}
	wantOrder := []string{"bkv_3", "bkv_2", "bkv_1"}
	if len(got) != len(wantOrder) {
		t.Fatalf("ListBackupVerifications() = %d rows, want %d", len(got), len(wantOrder))
	}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Errorf("row %d ID = %q, want %q", i, got[i].ID, id)
		}
	}

	latest, err := db.GetLatestBackupVerification(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetLatestBackupVerification() error = %v", err)
	}
	if latest.ID != "bkv_3" {
		t.Errorf("GetLatestBackupVerification() ID = %q, want %q", latest.ID, "bkv_3")
	}
}
