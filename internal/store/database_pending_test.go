package store

import (
	"context"
	"testing"
)

func TestPendingDatabase_SkippedByBackupSchedulerUntilReady(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "fresh", Engine: EnginePostgres, Version: "16", ExistingVolume: "reuse"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseBackupSchedule(ctx, "fresh", target.ID, "0 3 * * *", 7, 0); err != nil {
		t.Fatal(err)
	}
	id, err := db.BeginDatabaseProvisionAttempt(ctx, "fresh", 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AppendDatabaseProvisionEvent(ctx, DatabaseProvisionEvent{Database: "fresh", AttemptID: id, Step: "accepted", Status: "done"}); err != nil {
		t.Fatal(err)
	}

	pending, err := db.PendingDatabaseNames(ctx)
	if err != nil || !pending["fresh"] {
		t.Fatalf("pending = %v, err = %v, want fresh pending", pending, err)
	}
	got, err := db.ListScheduledDatabases(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("scheduler list = %+v, err = %v, want empty while pending", got, err)
	}
	saved, err := db.GetDesiredDatabase(ctx, "fresh")
	if err != nil || saved.ExistingVolume != "reuse" || saved.VolumeResolved {
		t.Fatalf("saved = %+v, err = %v, want reuse choice kept and volume unresolved", saved, err)
	}

	if err := db.AppendDatabaseProvisionEvent(ctx, DatabaseProvisionEvent{Database: "fresh", AttemptID: id, Step: "ready", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	got, err = db.ListScheduledDatabases(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("scheduler list = %+v, err = %v, want fresh once ready", got, err)
	}
}

func TestDeleteDesiredDatabase_ClearsProvisionEvents(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "gone", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatal(err)
	}
	id, _ := db.BeginDatabaseProvisionAttempt(ctx, "gone", 5)
	_ = db.AppendDatabaseProvisionEvent(ctx, DatabaseProvisionEvent{Database: "gone", AttemptID: id, Step: "accepted", Status: "done"})
	if err := db.DeleteDesiredDatabase(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := db.LatestDatabaseProvisionAttempt(ctx, "gone"); got != "" {
		t.Fatalf("attempt %q survived the delete", got)
	}
}
