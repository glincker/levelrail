package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVolumeBackupPolicy_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.GetVolumeBackupPolicy(ctx, "web", "data"); !errors.Is(err, ErrVolumeBackupPolicyNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	want := VolumeBackupPolicy{ServiceName: "web", VolumeName: "data", RetainDaily: 7, RetainWeekly: 4, RetainMonthly: 6, PreHook: "sync", PostHook: "true", Quiesce: VolumeQuiescePause}
	if err := db.SetVolumeBackupPolicy(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetVolumeBackupPolicy(ctx, "web", "data")
	if err != nil {
		t.Fatal(err)
	}
	want.UpdatedAt = got.UpdatedAt
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	want.RetainDaily = 1
	if err := db.SetVolumeBackupPolicy(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetVolumeBackupPolicy(ctx, "web", "data"); got.RetainDaily != 1 {
		t.Fatalf("update not applied: %+v", got)
	}
	if err := db.SetVolumeBackupPolicy(ctx, VolumeBackupPolicy{ServiceName: "web", VolumeName: "x", Quiesce: "freeze"}); err == nil {
		t.Fatal("an unknown quiesce mode must be rejected by the schema")
	}
}

func TestBackupSealInfoAndListings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)
	for i, id := range []string{"v1", "v2"} {
		start := time.Date(2026, 8, 14+i, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if err := db.StartBackupHistory(ctx, BackupHistory{ID: id, ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", TargetID: target.ID, ObjectKey: "k/" + id, StartedAt: start}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetBackupSealInfo(ctx, id, "zstd+age", "digest-"+id, 3, 99); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishBackupHistory(ctx, id, BackupStatusSucceeded, 10, "sum", "", start); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.StartBackupHistory(ctx, BackupHistory{ID: "run", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", TargetID: target.ID, ObjectKey: "k/run", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListSucceededVolumeBackups(ctx, "web", "data")
	if err != nil || len(rows) != 2 || rows[0].ID != "v2" || rows[0].Codec != "zstd+age" || rows[0].ManifestDigest != "digest-v2" || rows[0].ManifestFiles != 3 {
		t.Fatalf("rows = %+v err = %v", rows, err)
	}
	all, err := db.ListSucceededBackups(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("succeeded = %d err = %v", len(all), err)
	}
	stuck, err := db.ListRunningBackups(ctx, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(stuck) != 1 || stuck[0].ID != "run" {
		t.Fatalf("stuck = %+v err = %v", stuck, err)
	}
	if err := db.SetBackupSealInfo(ctx, "nope", "zstd", "", 0, 0); !errors.Is(err, ErrBackupHistoryNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestBackupDrills_RoundTripAndFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	d := BackupDrill{ID: "bkd_1", BackupHistoryID: "v1", ResourceKind: BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", Trigger: DrillTriggerScheduled, StartedAt: "2026-08-15T00:00:00Z"}
	if err := db.StartBackupDrill(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Status, d.Stage, d.ObjectOK, d.ChecksumOK, d.RestoreOK, d.ContentOK, d.Files, d.Bytes, d.DurationMS, d.FinishedAt = DrillStatusPassed, "done", true, true, true, true, 3, 99, 1200, "2026-08-15T00:00:02Z"
	if err := db.FinishBackupDrill(ctx, d); err != nil {
		t.Fatal(err)
	}
	other := BackupDrill{ID: "bkd_2", BackupHistoryID: "d1", ResourceKind: BackupResourceKindDatabase, DatabaseName: "main", StartedAt: "2026-08-16T00:00:00Z"}
	if err := db.StartBackupDrill(ctx, other); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetBackupDrill(ctx, "bkd_1")
	if err != nil || got != d {
		t.Fatalf("got %+v err %v want %+v", got, err, d)
	}
	vol, _ := db.ListBackupDrills(ctx, "web", "data", "", 10)
	if len(vol) != 1 || vol[0].ID != "bkd_1" {
		t.Fatalf("volume filter = %+v", vol)
	}
	dbs, _ := db.ListBackupDrills(ctx, "", "", "main", 10)
	if len(dbs) != 1 || dbs[0].ID != "bkd_2" || dbs[0].Trigger != DrillTriggerManual {
		t.Fatalf("database filter = %+v", dbs)
	}
	all, _ := db.ListBackupDrills(ctx, "", "", "", 10)
	if len(all) != 2 || all[0].ID != "bkd_2" {
		t.Fatalf("newest first = %+v", all)
	}
	if err := db.FinishBackupDrill(ctx, BackupDrill{ID: "missing"}); err == nil {
		t.Fatal("finishing an unknown drill must fail")
	}
}

func TestBackupTargetProtection_Upsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p := BackupTargetProtection{TargetID: "bkt_1", Versioning: "Enabled", CanDelete: true, CheckedAt: "2026-08-15T00:00:00Z"}
	if err := db.SetBackupTargetProtection(ctx, p); err != nil {
		t.Fatal(err)
	}
	p.ObjectLock, p.LockMode = true, "COMPLIANCE"
	if err := db.SetBackupTargetProtection(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListBackupTargetProtection(ctx)
	if err != nil || len(got) != 1 || got[0] != p {
		t.Fatalf("got %+v err %v", got, err)
	}
}
