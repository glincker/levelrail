package store

import (
	"context"
	"errors"
	"testing"
)

func TestMajorUpgradeLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u := MajorUpgrade{ID: "mu_1", DatabaseName: "main", FromVersion: "16", ToVersion: "17", StartedAt: "2026-10-05T00:00:00Z"}
	if err := db.StartMajorUpgrade(ctx, u); err != nil {
		t.Fatalf("StartMajorUpgrade() error = %v", err)
	}
	if running, err := db.ListRunningMajorUpgrades(ctx); err != nil || len(running) != 1 {
		t.Fatalf("ListRunningMajorUpgrades() = %v, %v, want one", running, err)
	}
	if err := db.UpdateMajorUpgradePhase(ctx, "mu_1", "snapshot", "db-main-data-pre16-mu1"); err != nil {
		t.Fatalf("UpdateMajorUpgradePhase() error = %v", err)
	}
	if err := db.UpdateMajorUpgradePhase(ctx, "mu_1", "restore", ""); err != nil {
		t.Fatalf("UpdateMajorUpgradePhase() keep snapshot error = %v", err)
	}
	got, err := db.GetMajorUpgrade(ctx, "mu_1")
	if err != nil {
		t.Fatalf("GetMajorUpgrade() error = %v", err)
	}
	if got.Phase != "restore" || got.SnapshotVolume != "db-main-data-pre16-mu1" || got.Status != BackupStatusRunning {
		t.Errorf("got %+v, want phase restore, snapshot kept, status running", got)
	}

	snaps, err := db.ListMajorUpgradeSnapshots(ctx)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("ListMajorUpgradeSnapshots() = %v, %v, want one", snaps, err)
	}

	if err := db.FinishMajorUpgrade(ctx, "mu_1", BackupStatusSucceeded, "", "2026-10-05T00:05:00Z"); err != nil {
		t.Fatalf("FinishMajorUpgrade() error = %v", err)
	}
	if err := db.ClearMajorUpgradeSnapshot(ctx, "mu_1"); err != nil {
		t.Fatalf("ClearMajorUpgradeSnapshot() error = %v", err)
	}
	if snaps, _ := db.ListMajorUpgradeSnapshots(ctx); len(snaps) != 0 {
		t.Errorf("snapshots after clear = %v, want none", snaps)
	}
	list, err := db.ListMajorUpgrades(ctx, "main")
	if err != nil || len(list) != 1 || list[0].Status != BackupStatusSucceeded {
		t.Errorf("ListMajorUpgrades() = %+v, %v", list, err)
	}

	if _, err := db.GetMajorUpgrade(ctx, "missing"); !errors.Is(err, ErrMajorUpgradeNotFound) {
		t.Errorf("GetMajorUpgrade(missing) error = %v, want ErrMajorUpgradeNotFound", err)
	}
	if err := db.FinishMajorUpgrade(ctx, "missing", BackupStatusFailed, "", ""); !errors.Is(err, ErrMajorUpgradeNotFound) {
		t.Errorf("FinishMajorUpgrade(missing) error = %v", err)
	}
}
