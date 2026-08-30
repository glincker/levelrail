package store

import (
	"context"
	"errors"
	"testing"
)

func newTestBackupTarget() BackupTarget {
	return BackupTarget{
		ID:        "bkt_test1",
		Name:      "primary backups",
		Provider:  BackupProviderR2,
		Endpoint:  "https://example.r2.cloudflarestorage.com",
		Region:    "auto",
		Bucket:    "levelrail-backups",
		CreatedAt: "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, want); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}

	got, err := db.GetBackupTarget(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	if got != want {
		t.Errorf("GetBackupTarget() = %+v, want %+v", got, want)
	}
}

func TestGetBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetBackupTarget(ctx, "bkt_missing")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("GetBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestListBackupTargets_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestBackupTarget()
	first.ID, first.CreatedAt = "bkt_a", "2026-08-14T00:00:00Z"
	second := newTestBackupTarget()
	second.ID, second.CreatedAt = "bkt_b", "2026-08-14T00:00:01Z"

	if err := db.SaveBackupTarget(ctx, second); err != nil {
		t.Fatalf("SaveBackupTarget(second) error = %v", err)
	}
	if err := db.SaveBackupTarget(ctx, first); err != nil {
		t.Fatalf("SaveBackupTarget(first) error = %v", err)
	}

	got, err := db.ListBackupTargets(ctx)
	if err != nil {
		t.Fatalf("ListBackupTargets() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "bkt_a" || got[1].ID != "bkt_b" {
		t.Fatalf("ListBackupTargets() = %+v, want [bkt_a, bkt_b] in creation order", got)
	}
}

func TestUpdateBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}

	if err := db.UpdateBackupTarget(ctx, target.ID, "renamed", BackupProviderAWS, "", "us-east-1", "new-bucket"); err != nil {
		t.Fatalf("UpdateBackupTarget() error = %v", err)
	}

	got, err := db.GetBackupTarget(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetBackupTarget() error = %v", err)
	}
	want := BackupTarget{ID: target.ID, Name: "renamed", Provider: BackupProviderAWS, Endpoint: "", Region: "us-east-1", Bucket: "new-bucket", CreatedAt: target.CreatedAt}
	if got != want {
		t.Errorf("GetBackupTarget() after update = %+v, want %+v", got, want)
	}
}

func TestUpdateBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateBackupTarget(ctx, "bkt_missing", "x", BackupProviderAWS, "", "us-east-1", "bucket")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("UpdateBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, want); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	if err := db.DeleteBackupTarget(ctx, want.ID); err != nil {
		t.Fatalf("DeleteBackupTarget() error = %v", err)
	}

	_, err := db.GetBackupTarget(ctx, want.ID)
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("GetBackupTarget() after delete error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteBackupTarget(ctx, "bkt_missing")
	if !errors.Is(err, ErrBackupTargetNotFound) {
		t.Fatalf("DeleteBackupTarget() error = %v, want ErrBackupTargetNotFound", err)
	}
}

func TestDeleteBackupTarget_ReferencedByHistory_FailsCleanly(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := newTestBackupTarget()
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: target.ID,
		ObjectKey: "mydb/mydb-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	// Foreign key enforcement (store.go's "_pragma foreign_keys(ON)")
	// must reject this, not silently orphan or cascade-delete the
	// history row: see DeleteBackupTarget's own doc comment.
	if err := db.DeleteBackupTarget(ctx, target.ID); err == nil {
		t.Fatal("DeleteBackupTarget() error = nil, want a foreign key constraint error while backup_history still references this target")
	}
}
