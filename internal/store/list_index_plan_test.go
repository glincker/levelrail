package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// planFor returns the EXPLAIN QUERY PLAN detail lines for query.
func planFor(t *testing.T, db *DB, query string, args ...any) string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain %q: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	return strings.Join(lines, "\n")
}

func seedPlanRows(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 600; i++ {
		ts := fmt.Sprintf("2026-01-01T00:%02d:%02dZ", i/60, i%60)
		if _, err := db.ExecContext(ctx, `INSERT INTO audit_log (actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, client_kind) VALUES ('session', ?, ?, 'read', ?, ?, 200, '1.1.1.1', ?, ?)`,
			fmt.Sprintf("u%d", i%7), fmt.Sprintf("name%d", i%7), []string{"GET", "POST"}[i%2], fmt.Sprintf("/p/%d", i%11), ts, []string{"web", "cli"}[i%2]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `ANALYZE`); err != nil {
		t.Fatal(err)
	}
}

func TestListQueriesUseCompositeIndexes(t *testing.T) {
	db := openTestDB(t)
	seedPlanRows(t, db)
	cases := []struct{ name, query, index string }{
		{"audit_actor", `SELECT id FROM audit_log WHERE actor_name = 'name1' AND created_at < '2026-02-01' ORDER BY created_at DESC LIMIT 50`, "idx_audit_log_actor_name_created"},
		{"audit_owner", `SELECT id FROM audit_log WHERE actor_type = 'session' AND actor_id = 'u1' ORDER BY created_at DESC LIMIT 50`, "idx_audit_log_actor_id_created"},
		{"deploy_service", `SELECT id FROM deploy_attempts WHERE service_name = 'a' ORDER BY started_at DESC, id DESC LIMIT 50`, "idx_deploy_attempts_service_started_id"},
	}
	for _, c := range cases {
		plan := planFor(t, db, c.query)
		if !strings.Contains(plan, c.index) || strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("%s plan = %q, want %s without a temp b-tree", c.name, plan, c.index)
		}
	}
}
