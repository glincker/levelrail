package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSelfUpgradeAttemptUpsertAndList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	running := SelfUpgradeAttempt{
		ID: "su-1", FromVersion: "v1.0.0", ToVersion: "v1.1.0", FromSchema: 10, ToSchema: 11,
		Initiator: "dashboard:admin", Outcome: "running", StartedAt: "2026-10-10T10:00:00.000Z",
	}
	if err := db.UpsertSelfUpgradeAttempt(ctx, running); err != nil {
		t.Fatal(err)
	}
	done := running
	done.Outcome, done.FailedStep, done.Error = "rolled_back", "health", "not healthy"
	done.StepsJSON, done.FinishedAt = `[{"name":"health","status":"failed"}]`, "2026-10-10T10:01:00.000Z"
	if err := db.UpsertSelfUpgradeAttempt(ctx, done); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSelfUpgradeAttempt(ctx, SelfUpgradeAttempt{
		ID: "su-2", FromVersion: "v1.1.0", ToVersion: "v1.2.0", Outcome: "succeeded", StartedAt: "2026-10-11T10:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.ListSelfUpgradeAttempts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "su-2" {
		t.Fatalf("want newest first and an upsert not a duplicate, got %+v", got)
	}
	if got[1].Outcome != "rolled_back" || got[1].FailedStep != "health" || got[1].FinishedAt == "" {
		t.Fatalf("upsert did not replace the running row: %+v", got[1])
	}
	if got[0].AckedJSON != "[]" || got[0].StepsJSON != "[]" {
		t.Fatalf("empty JSON columns not defaulted: %+v", got[0])
	}
}

func TestSelfUpgradeAttemptRejectsUnknownOutcome(t *testing.T) {
	db := openTestDB(t)
	err := db.UpsertSelfUpgradeAttempt(context.Background(), SelfUpgradeAttempt{
		ID: "su-x", FromVersion: "v1", ToVersion: "v2", Outcome: "weird", StartedAt: "2026-10-10T10:00:00.000Z",
	})
	if err == nil {
		t.Fatal("outcome outside the CHECK constraint was accepted")
	}
}

func TestReadSchemaState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES (99999, 'future')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO upgrade_history (id, kind, from_version, to_version, channel, schema_before, schema_after,
		occurred_at, initiator, method, backup_name, health, notes, notes_state)
		VALUES ('uh_1', 'upgraded', 'v1', 'v9.9.9', 'stable', 1, 99999, '2026-10-10T10:00:00.000Z', 'x', '', '', 'booted', '', 'unavailable')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := ReadSchemaState(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Schema != 99999 || st.LastVersion != "v9.9.9" {
		t.Fatalf("state = %+v", st)
	}
	newest, err := MaxSchemaVersion()
	if err != nil || st.Schema <= newest {
		t.Fatalf("database should be newer than this binary: db %d, binary %d, %v", st.Schema, newest, err)
	}
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("a database newer than the binary opened")
	}
}
