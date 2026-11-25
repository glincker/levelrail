package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func auditRow(i int, db, principal, action, outcome string, at time.Time) DatabaseStatementAudit {
	return DatabaseStatementAudit{ID: fmt.Sprintf("a%03d", i), At: at.UTC().Format(time.RFC3339Nano), DatabaseName: db, Engine: "postgres", PrincipalType: "user", PrincipalID: principal, Action: action, OpKind: "rows", Outcome: outcome, BranchName: ""}
}

func TestDatabaseStatementAuditListFilterAndPage(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	var rows []DatabaseStatementAudit
	for i := 0; i < 5; i++ {
		rows = append(rows, auditRow(i, "app", "u1", "db:read", "ok", now.Add(time.Duration(i)*time.Second)))
	}
	rows = append(rows, auditRow(10, "app", "u2", "db:write", "denied", now.Add(10*time.Second)), auditRow(11, "other", "u1", "db:read", "ok", now))
	rows[0].BranchName = "feat"
	if err := db.InsertDatabaseStatementAudits(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, _, err := db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app"})
	if err != nil || len(got) != 6 {
		t.Fatalf("list app: %d %v", len(got), err)
	}
	got, _, _ = db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app", PrincipalID: "u2"})
	if len(got) != 1 || got[0].Outcome != "denied" {
		t.Fatalf("principal filter: %+v", got)
	}
	got, _, _ = db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app", Action: "db:read", Outcome: "ok", Since: now.Add(2 * time.Second).UTC().Format(time.RFC3339Nano)})
	if len(got) != 3 {
		t.Fatalf("time and action filter: %d", len(got))
	}
	page, next, err := db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app", Limit: 4})
	if err != nil || len(page) != 4 || next == "" {
		t.Fatalf("page 1: %d %q %v", len(page), next, err)
	}
	page2, next2, err := db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app", Limit: 4, Cursor: next})
	if err != nil || len(page2) != 2 || next2 != "" {
		t.Fatalf("page 2: %d %q %v", len(page2), next2, err)
	}
	if _, _, err := db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app", Cursor: "bad"}); err == nil {
		t.Fatal("bad cursor must fail")
	}
}

func TestDatabaseStatementAuditPrune(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	var rows []DatabaseStatementAudit
	for i := 0; i < 7; i++ {
		rows = append(rows, auditRow(i, "app", "u1", "db:read", "ok", now.Add(-48*time.Hour)))
	}
	rows = append(rows, auditRow(50, "app", "u1", "db:read", "ok", now))
	if err := db.InsertDatabaseStatementAudits(ctx, rows); err != nil {
		t.Fatal(err)
	}
	n, err := db.PruneDatabaseStatementAudits(ctx, now.Add(-24*time.Hour), 3)
	if err != nil || n != 7 {
		t.Fatalf("prune: %d %v", n, err)
	}
	got, _, _ := db.ListDatabaseStatementAudits(ctx, DatabaseStatementAuditFilter{DatabaseName: "app"})
	if len(got) != 1 {
		t.Fatalf("left: %d", len(got))
	}
}
