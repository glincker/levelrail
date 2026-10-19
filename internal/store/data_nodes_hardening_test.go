package store

import (
	"context"
	"testing"
)

func TestBackupTombstoneLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.AddBackupTombstone(ctx, "t1", "db/a.dump", "boom"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddBackupTombstone(ctx, "t1", "db/a.dump", "boom again"); err != nil {
		t.Fatal(err)
	}
	n, err := db.CountBackupTombstones(ctx)
	if err != nil || n != 1 {
		t.Fatalf("repeat add must not duplicate: n=%d err=%v", n, err)
	}
	list, err := db.ListBackupTombstones(ctx, 10)
	if err != nil || len(list) != 1 || list[0].Reason != "boom again" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := db.BumpBackupTombstone(ctx, list[0].ID, "still down"); err != nil {
		t.Fatal(err)
	}
	list, _ = db.ListBackupTombstones(ctx, 10)
	if list[0].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", list[0].Attempts)
	}
	if err := db.ResolveBackupTombstone(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountBackupTombstones(ctx); n != 0 {
		t.Fatalf("want 0 after resolve, got %d", n)
	}
}

func TestDatabaseTeardownJobLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDatabaseTeardownJob(ctx, "main", "node-1", false); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDatabaseTeardownJob(ctx, "main", "node-1", true); err != nil {
		t.Fatal(err)
	}
	jobs, err := db.ListDatabaseTeardownJobs(ctx)
	if err != nil || len(jobs) != 1 || !jobs[0].Purge {
		t.Fatalf("a later purge request must upgrade the job: %+v err=%v", jobs, err)
	}
	if err := db.FailDatabaseTeardownJob(ctx, "main", "node-1", "docker down"); err != nil {
		t.Fatal(err)
	}
	jobs, _ = db.ListDatabaseTeardownJobs(ctx)
	if jobs[0].Attempts != 1 || jobs[0].LastError != "docker down" {
		t.Fatalf("failure not recorded: %+v", jobs[0])
	}
	if err := db.FinishDatabaseTeardownJob(ctx, "main", "node-1"); err != nil {
		t.Fatal(err)
	}
	if jobs, _ = db.ListDatabaseTeardownJobs(ctx); len(jobs) != 0 {
		t.Fatalf("finished job must be gone: %+v", jobs)
	}
}

func TestListNewestSucceededBackupsOnePerResource(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := seedBackupTarget(t, db)
	vol := func(id, svc, at string) BackupHistory {
		return BackupHistory{ID: id, ResourceKind: BackupResourceKindVolume, ServiceName: svc, VolumeName: "v", TargetID: target.ID, ObjectKey: "k/" + id, StartedAt: at}
	}
	for _, h := range []BackupHistory{
		vol("a1", "a", "2026-10-01T00:00:00Z"),
		vol("a2", "a", "2026-10-02T00:00:00Z"),
		vol("b1", "b", "2026-10-01T00:00:00Z"),
	} {
		if err := db.StartBackupHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishBackupHistory(ctx, h.ID, BackupStatusSucceeded, 1, "sum", "", "2026-10-03T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.StartBackupHistory(ctx, vol("a3", "a", "2026-10-04T00:00:00Z")); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListNewestSucceededBackups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if len(rows) != 2 || !got["a2"] || !got["b1"] {
		t.Fatalf("want newest succeeded per database (a2, b1), got %+v", got)
	}
}
