package store

import (
	"context"
	"errors"
	"testing"
)

func seedVolumeCloneRestoreBackup(t *testing.T, db *DB) BackupHistory {
	t.Helper()
	target := seedBackupTarget(t, db)
	h := BackupHistory{
		ID: "bkh_1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}
	if err := db.StartBackupHistory(context.Background(), h); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), h.ID, BackupStatusSucceeded, 1, "", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
	return h
}

func TestStartAndFinishVolumeCloneRestore_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedVolumeCloneRestoreBackup(t, db)

	if err := db.StartVolumeCloneRestore(ctx, VolumeCloneRestore{
		ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data", NewVolumeName: "clone-web-data-abc123",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartVolumeCloneRestore() error = %v", err)
	}

	got := mustListOneVolumeCloneRestore(t, db, "web", "data")
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}
	if got.NewVolumeName != "clone-web-data-abc123" {
		t.Fatalf("NewVolumeName = %q, want %q", got.NewVolumeName, "clone-web-data-abc123")
	}

	if err := db.FinishVolumeCloneRestore(ctx, "vcr_1", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z"); err != nil {
		t.Fatalf("FinishVolumeCloneRestore() error = %v", err)
	}

	got = mustListOneVolumeCloneRestore(t, db, "web", "data")
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

func TestFinishVolumeCloneRestore_Failed_RecordsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedVolumeCloneRestoreBackup(t, db)

	if err := db.StartVolumeCloneRestore(ctx, VolumeCloneRestore{
		ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data", NewVolumeName: "clone-web-data-abc123",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z",
	}); err != nil {
		t.Fatalf("StartVolumeCloneRestore() error = %v", err)
	}

	if err := db.FinishVolumeCloneRestore(ctx, "vcr_1", BackupStatusFailed, "docker daemon unreachable", "2026-08-14T01:05:00Z"); err != nil {
		t.Fatalf("FinishVolumeCloneRestore() error = %v", err)
	}

	got := mustListOneVolumeCloneRestore(t, db, "web", "data")
	if got.Status != BackupStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, BackupStatusFailed)
	}
	if got.Error != "docker daemon unreachable" {
		t.Errorf("Error = %q, want the failure reason", got.Error)
	}
}

func TestFinishVolumeCloneRestore_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.FinishVolumeCloneRestore(ctx, "vcr_missing", BackupStatusSucceeded, "", "2026-08-14T01:05:00Z")
	if !errors.Is(err, ErrVolumeCloneRestoreNotFound) {
		t.Fatalf("FinishVolumeCloneRestore() error = %v, want ErrVolumeCloneRestoreNotFound", err)
	}
}

func TestListVolumeCloneRestores_NewestFirst_ScopedToSourceServiceAndVolume(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	backup := seedVolumeCloneRestoreBackup(t, db)

	for _, h := range []VolumeCloneRestore{
		{ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data", NewVolumeName: "clone-a", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:00:00Z"},
		{ID: "vcr_2", SourceServiceName: "web", SourceVolumeName: "data", NewVolumeName: "clone-b", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:01:00Z"},
		{ID: "vcr_3", SourceServiceName: "web", SourceVolumeName: "cache", NewVolumeName: "clone-c", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:02:00Z"},
		{ID: "vcr_4", SourceServiceName: "api", SourceVolumeName: "data", NewVolumeName: "clone-d", BackupHistoryID: backup.ID, StartedAt: "2026-08-14T01:03:00Z"},
	} {
		if err := db.StartVolumeCloneRestore(ctx, h); err != nil {
			t.Fatalf("StartVolumeCloneRestore(%s) error = %v", h.ID, err)
		}
	}

	got, err := db.ListVolumeCloneRestores(ctx, "web", "data")
	if err != nil {
		t.Fatalf("ListVolumeCloneRestores() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "vcr_2" || got[1].ID != "vcr_1" {
		t.Fatalf("ListVolumeCloneRestores(web, data) = %+v, want [vcr_2, vcr_1] newest first, other service/volume rows excluded", got)
	}
}

func mustListOneVolumeCloneRestore(t *testing.T, db *DB, sourceServiceName, sourceVolumeName string) VolumeCloneRestore {
	t.Helper()
	got, err := db.ListVolumeCloneRestores(context.Background(), sourceServiceName, sourceVolumeName)
	if err != nil {
		t.Fatalf("ListVolumeCloneRestores(%q, %q) error = %v", sourceServiceName, sourceVolumeName, err)
	}
	if len(got) != 1 {
		t.Fatalf("ListVolumeCloneRestores(%q, %q) = %+v, want exactly 1 row", sourceServiceName, sourceVolumeName, got)
	}
	return got[0]
}
