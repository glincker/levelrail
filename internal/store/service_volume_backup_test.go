package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func mustListOneVolume(t *testing.T, db *DB, serviceName, volumeName string) BackupHistory {
	t.Helper()
	got, err := db.ListServiceVolumeBackupHistory(context.Background(), serviceName, volumeName, 50, nil)
	if err != nil {
		t.Fatalf("ListServiceVolumeBackupHistory(%q, %q) error = %v", serviceName, volumeName, err)
	}
	if len(got) != 1 {
		t.Fatalf("ListServiceVolumeBackupHistory(%q, %q) = %+v, want exactly 1 row", serviceName, volumeName, got)
	}
	return got[0]
}

func TestStartAndFinishBackupHistory_Volume(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	got := mustListOneVolume(t, db, "web", "data")
	if got.Status != BackupStatusRunning {
		t.Fatalf("status after Start = %q, want %q", got.Status, BackupStatusRunning)
	}
	if got.DatabaseName != "" {
		t.Errorf("DatabaseName = %q, want empty for a volume backup row", got.DatabaseName)
	}

	if err := db.FinishBackupHistory(ctx, "bkh_1", BackupStatusSucceeded, 2048, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}

	got = mustListOneVolume(t, db, "web", "data")
	if got.Status != BackupStatusSucceeded || got.SizeBytes != 2048 || got.ChecksumSHA256 != "sum" {
		t.Errorf("finished row = %+v, want succeeded/2048/sum", got)
	}

	// A database-scoped list must never pick up a volume row and vice
	// versa: the whole point of ResourceKind is to keep the two
	// identities from ever bleeding into each other's queries.
	dbRows, err := db.ListBackupHistory(ctx, "web", 50, nil)
	if err != nil {
		t.Fatalf("ListBackupHistory() error = %v", err)
	}
	if len(dbRows) != 0 {
		t.Errorf("ListBackupHistory(%q) = %+v, want 0 rows (that row is a volume backup, not a database one)", "web", dbRows)
	}
}

func TestGetBackupHistory_Volume(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.StartBackupHistory(ctx, BackupHistory{
		ID: "bkh_1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}

	got, err := db.GetBackupHistory(ctx, "bkh_1")
	if err != nil {
		t.Fatalf("GetBackupHistory() error = %v", err)
	}
	if got.ResourceKind != BackupResourceKindVolume || got.ServiceName != "web" || got.VolumeName != "data" {
		t.Errorf("got = %+v, want ResourceKind=volume ServiceName=web VolumeName=data", got)
	}
}

func TestPruneServiceVolumeBackupHistory_KeepsNewest(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	for i, ts := range []string{"2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z", "2026-08-03T00:00:00Z"} {
		id := "bkh_" + string(rune('a'+i))
		if err := db.StartBackupHistory(ctx, BackupHistory{
			ID: id, ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
			TargetID: target.ID, ObjectKey: "volumes/web/data/" + id + ".tar", StartedAt: ts,
		}); err != nil {
			t.Fatalf("StartBackupHistory(%q) error = %v", id, err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 10, "sum", "", ts); err != nil {
			t.Fatalf("FinishBackupHistory(%q) error = %v", id, err)
		}
	}

	pruned, err := db.PruneServiceVolumeBackupHistory(ctx, "web", "data", 1, time.Time{})
	if err != nil {
		t.Fatalf("PruneServiceVolumeBackupHistory() error = %v", err)
	}
	if len(pruned) != 2 {
		t.Fatalf("pruned = %+v, want 2 rows removed (keeping only the newest)", pruned)
	}

	remaining, err := db.ListServiceVolumeBackupHistory(ctx, "web", "data", 50, nil)
	if err != nil {
		t.Fatalf("ListServiceVolumeBackupHistory() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "bkh_c" {
		t.Errorf("remaining = %+v, want only bkh_c (the newest)", remaining)
	}
}

func TestServiceVolumeBackupSchedule_SetGetClear(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if _, err := db.GetServiceVolumeBackupSchedule(ctx, "web", "data"); !errors.Is(err, ErrServiceVolumeBackupNotFound) {
		t.Fatalf("GetServiceVolumeBackupSchedule() before any Set error = %v, want ErrServiceVolumeBackupNotFound", err)
	}

	if err := db.SetServiceVolumeBackupSchedule(ctx, "web", "data", target.ID, "0 3 * * *", 7, 30); err != nil {
		t.Fatalf("SetServiceVolumeBackupSchedule() error = %v", err)
	}

	got, err := db.GetServiceVolumeBackupSchedule(ctx, "web", "data")
	if err != nil {
		t.Fatalf("GetServiceVolumeBackupSchedule() error = %v", err)
	}
	if got.BackupTargetID != target.ID || got.BackupSchedule != "0 3 * * *" || got.BackupRetain != 7 || got.BackupRetainDays != 30 {
		t.Errorf("got = %+v, want the values just set", got)
	}

	scheduled, err := db.ListScheduledServiceVolumes(ctx)
	if err != nil {
		t.Fatalf("ListScheduledServiceVolumes() error = %v", err)
	}
	if len(scheduled) != 1 || scheduled[0].ServiceName != "web" || scheduled[0].VolumeName != "data" {
		t.Errorf("scheduled = %+v, want one entry for web/data", scheduled)
	}

	// Re-setting (upsert) with target/schedule cleared is how the API
	// layer implements "clear the schedule": ListScheduledServiceVolumes
	// must then stop returning it, the same "" sentinel
	// SetDatabaseBackupSchedule already establishes for databases.
	if err := db.SetServiceVolumeBackupSchedule(ctx, "web", "data", "", "", 0, 0); err != nil {
		t.Fatalf("SetServiceVolumeBackupSchedule() clear error = %v", err)
	}
	scheduled, err = db.ListScheduledServiceVolumes(ctx)
	if err != nil {
		t.Fatalf("ListScheduledServiceVolumes() after clear error = %v", err)
	}
	if len(scheduled) != 0 {
		t.Errorf("scheduled after clear = %+v, want 0 entries", scheduled)
	}
}
