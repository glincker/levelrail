package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration3342_KeepsExistingAuditRowsReadableByName(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3341))
	if err != nil {
		t.Fatalf("open at 3341: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO database_statement_audit (id, at, database_name, action, outcome) VALUES ('legacy', '2026-11-01T00:00:00.000000000Z', 'main', 'db:read', 'ok')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()
	rows, _, err := cur.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "main", DatabaseID: "inst-1", MatchDatabaseID: true})
	if err != nil || len(rows) != 1 || rows[0].ID != "legacy" {
		t.Fatalf("rows %+v %v, want the legacy row", rows, err)
	}
}
