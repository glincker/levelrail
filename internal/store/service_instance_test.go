package store

import (
	"context"
	"testing"
)

func TestVolumeBackupOwnership_ReusedServiceNameDoesNotInherit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t)
	tg := seedBackupTarget(t, db)
	row := BackupHistory{TargetID: tg.ID, ID: "v1", ResourceKind: BackupResourceKindVolume, ServiceName: "wiki", VolumeName: "data", StartedAt: "2026-08-14T00:00:00Z"}
	if err := db.StartBackupHistory(ctx, row); err != nil {
		t.Fatal(err)
	}
	old, err := db.GetBackupHistory(ctx, "v1")
	if err != nil || old.InstanceID == "" {
		t.Fatalf("row = %+v, err = %v, want a stamped instance", old, err)
	}
	if ok, _ := db.OwnsVolumeBackup(ctx, "wiki", old.InstanceID); !ok {
		t.Fatal("original app does not own its backup")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM service_instances WHERE name = 'wiki'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ServiceInstanceID(ctx, "wiki"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := db.OwnsVolumeBackup(ctx, "wiki", old.InstanceID); ok {
		t.Fatal("a new app of the same name owns the old backup")
	}
	if rows, _ := db.ListServiceVolumeBackupHistory(ctx, "wiki", "data", 10, nil); len(rows) != 0 {
		t.Fatalf("newcomer list = %+v, want none", rows)
	}
	if rows, _ := db.ListServiceVolumeBackupHistory(WithLegacyBackups(ctx), "wiki", "data", 10, nil); len(rows) != 0 {
		t.Fatalf("stamped foreign row listed as legacy: %+v", rows)
	}
}
