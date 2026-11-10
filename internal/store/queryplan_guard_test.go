package store

import (
	"context"
	"strings"
	"testing"
)

// hotReadQueries mirrors the shape of the read paths dashboards poll. A new
// full scan here means a missing index, which only shows at scale.
var hotReadQueries = []struct {
	name, query string
	args        []any
}{
	{"deploy attempts by app, newest first", `SELECT id FROM deploy_attempts WHERE service_name = ? ORDER BY started_at DESC LIMIT 20`, []any{"web"}},
	{"deploy attempt by id", `SELECT id FROM deploy_attempts WHERE id = ?`, []any{"x"}},
	{"deploy timeline by attempt", `SELECT attempt_id FROM deploy_timelines WHERE attempt_id = ?`, []any{"x"}},
	{"app by name", `SELECT name FROM desired_services WHERE name = ?`, []any{"web"}},
	{"apps on a node", `SELECT name FROM desired_services WHERE node_id = ?`, []any{"n1"}},
	{"databases on a node", `SELECT name FROM desired_databases WHERE node_id = ?`, []any{"n1"}},
	{"domains by app", `SELECT service_name FROM service_domains WHERE service_name = ?`, []any{"web"}},
	{"backups by database, newest first", `SELECT id FROM backup_history WHERE database_name = ? ORDER BY started_at DESC LIMIT 20`, []any{"db"}},
	{"audit page by time cursor", `SELECT id FROM audit_log WHERE created_at < ? ORDER BY created_at DESC LIMIT 50`, []any{"2030-01-01T00:00:00Z"}},
}

func TestQueryPlanGuard(t *testing.T) {
	db := openTestDB(t)
	for _, q := range hotReadQueries {
		t.Run(q.name, func(t *testing.T) {
			rows, err := db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q.query, q.args...)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatalf("scan: %v", err)
				}
				if strings.HasPrefix(detail, "SCAN ") && !strings.Contains(detail, "USING") {
					t.Errorf("full table scan: %s", detail)
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
		})
	}
}
