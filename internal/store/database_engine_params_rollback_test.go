package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedAppliedParams(t *testing.T, db *DB, next map[string]string) DatabaseEngineParams {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "pg", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.StageDatabaseEngineParams(ctx, "pg", next, "alice", nil); err != nil {
		t.Fatalf("stage: %v", err)
	}
	p, _ := db.GetDatabaseEngineParams(ctx, "pg")
	if ok, err := db.ApplyDatabaseEngineParams(ctx, "pg", *p.PendingRaw); err != nil || !ok {
		t.Fatalf("apply = %v %v", ok, err)
	}
	p, _ = db.GetDatabaseEngineParams(ctx, "pg")
	if p.AppliedAt == "" {
		t.Fatal("apply did not record applied_at")
	}
	return p
}

func rollbackAudit() AuditEntry {
	id, _ := NewAuditEntryID()
	return AuditEntry{ID: id, ActorType: "system", ClientKind: "system", Action: AuditActionDatabaseParamsAutoRollback, CreatedAt: FormatAuditTime(time.Now())}
}

func TestDatabaseEngineParamsRollbackOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p := seedAppliedParams(t, db, map[string]string{"max_connections": "500"})

	// A different apply time means another apply happened since it was read.
	if rolled, err := db.RollbackDatabaseEngineParams(ctx, "pg", p.Applied, "2000-01-01T00:00:00Z", "NotAcceptingConnections", time.Now(), rollbackAudit()); err != nil || rolled {
		t.Fatalf("stale apply time rollback = %v %v, want none", rolled, err)
	}
	rolled, err := db.RollbackDatabaseEngineParams(ctx, "pg", p.Applied, p.AppliedAt, "NotAcceptingConnections", time.Now(), rollbackAudit())
	if err != nil || !rolled {
		t.Fatalf("rollback = %v %v", rolled, err)
	}
	after, _ := db.GetDatabaseEngineParams(ctx, "pg")
	if len(after.Applied) != 0 || after.Previous["max_connections"] != "500" || after.RolledBackAt == "" || after.RolledBackReason != "NotAcceptingConnections" {
		t.Fatalf("after rollback = %+v, want the failed set kept in previous", after)
	}
	// The restored set failing too must not flip back.
	if rolled, err := db.RollbackDatabaseEngineParams(ctx, "pg", after.Applied, after.AppliedAt, "NotAcceptingConnections", time.Now(), rollbackAudit()); err != nil || rolled {
		t.Fatalf("second rollback = %v %v, want none", rolled, err)
	}
	entries, err := db.ListAuditEntries(ctx, 10, nil, AuditEntryFilter{})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == AuditActionDatabaseParamsAutoRollback {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("auto rollback audit rows = %d, want 1", n)
	}
}

func TestSetDatabaseVersionDroppingEngineParamsIsAtomic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p := seedAppliedParams(t, db, map[string]string{"max_connections": "500", "work_mem": "8MB"})
	keep := map[string]string{"work_mem": "8MB"}
	audit := rollbackAudit()
	audit.Action = "database.params.drop_incompatible"

	// A version that moved meanwhile fails the whole write.
	if err := db.SetDatabaseVersionDroppingEngineParams(ctx, "pg", "15", "16.4", p.Applied, keep, audit); !errors.Is(err, ErrEngineParamsConflict) {
		t.Fatalf("stale version = %v, want conflict", err)
	}
	d, _ := db.GetDesiredDatabase(ctx, "pg")
	if got := d.EngineParams.Map(); d.Version != "16" || got["max_connections"] != "500" {
		t.Fatalf("failed write changed state: %s %v", d.Version, got)
	}
	if err := db.SetDatabaseVersionDroppingEngineParams(ctx, "pg", "16", "16.4", p.Applied, keep, audit); err != nil {
		t.Fatalf("drop: %v", err)
	}
	d, _ = db.GetDesiredDatabase(ctx, "pg")
	if got := d.EngineParams.Map(); d.Version != "16.4" || got["max_connections"] != "" || got["work_mem"] != "8MB" {
		t.Fatalf("after drop = %s %v", d.Version, got)
	}
}

func TestSetDatabaseVersionIfEngineParamsRefusesChangedSet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p := seedAppliedParams(t, db, map[string]string{"max_connections": "500"})
	if err := db.SetDatabaseVersionIfEngineParams(ctx, "pg", "16", "16.4", map[string]string{"max_connections": "200"}); !errors.Is(err, ErrEngineParamsConflict) {
		t.Fatalf("changed set = %v, want conflict", err)
	}
	if d, _ := db.GetDesiredDatabase(ctx, "pg"); d.Version != "16" {
		t.Fatalf("version changed on conflict: %s", d.Version)
	}
	if err := db.SetDatabaseVersionIfEngineParams(ctx, "pg", "16", "16.4", p.Applied); err != nil {
		t.Fatalf("set version: %v", err)
	}
	after, _ := db.GetDatabaseEngineParams(ctx, "pg")
	if d, _ := db.GetDesiredDatabase(ctx, "pg"); d.Version != "16.4" || after.Previous["max_connections"] != "" || after.Applied["max_connections"] != "500" {
		t.Fatalf("after = %s %+v", d.Version, after)
	}
}
