package store

import (
	"context"
	"errors"
	"testing"
)

func TestPredeploySnapshotSettingsAndRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s, err := db.GetPredeploySnapshotSettings(ctx, "web")
	if err != nil || s.Mode != PredeployModeOff {
		t.Fatalf("default settings = %+v, %v; want off", s, err)
	}
	if err := db.SetPredeploySnapshotSettings(ctx, "web", PredeploySnapshotSettings{Mode: PredeployModeRequire, TargetID: "bkt_1"}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	if s, _ = db.GetPredeploySnapshotSettings(ctx, "web"); s.Mode != PredeployModeRequire || s.TargetID != "bkt_1" {
		t.Fatalf("settings = %+v", s)
	}

	for i, id := range []string{"pds_a", "pds_b"} {
		err := db.SavePredeploySnapshot(ctx, PredeploySnapshot{
			ID: id, App: "web", PriorDeployID: "dep_0", DatabaseName: "main", Status: PredeploySnapshotSucceeded,
			BackupHistoryID: "bkh_" + id, CreatedAt: "2026-10-28T00:00:0" + string(rune('1'+i)) + "Z",
		})
		if err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	if err := db.AttachPredeploySnapshotsToDeploy(ctx, []string{"pds_b"}, "dep_1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	forDeploy, err := db.ListPredeploySnapshotsForDeploy(ctx, "web", "dep_1")
	if err != nil || len(forDeploy) != 1 || forDeploy[0].ID != "pds_b" {
		t.Fatalf("for deploy = %+v, %v", forDeploy, err)
	}
	all, err := db.ListPredeploySnapshots(ctx, "web")
	if err != nil || len(all) != 2 || all[0].ID != "pds_b" {
		t.Fatalf("list = %+v, %v; want newest first", all, err)
	}
	if err := db.SetPredeploySnapshotRestore(ctx, "pds_b", "main-rb-1", PredeployRestoreSucceeded, ""); err != nil {
		t.Fatalf("set restore: %v", err)
	}
	if err := db.SetPredeploySnapshotRestore(ctx, "missing", "x", PredeployRestoreFailed, "e"); !errors.Is(err, ErrPredeploySnapshotNotFound) {
		t.Fatalf("missing restore err = %v", err)
	}
	if err := db.DeletePredeploySnapshot(ctx, "pds_a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
