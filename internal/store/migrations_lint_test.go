package store

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	largeTableRebuildMarker = "-- migrate: large-table-rebuild-reviewed"
	// headerRequiredFrom is the first version whose file must open with a why comment.
	headerRequiredFrom = 2350
)

// unboundedTables grow with traffic or history, so rebuilding one (copy, drop,
// rename) holds the write lock for as long as the copy takes on a big install.
// Add a table here when it is append-heavy; a new migration that rebuilds one
// needs largeTableRebuildMarker plus a one-line justification.
var unboundedTables = []string{
	"ai_chat_messages", "ai_usage", "app_autoscale_events", "app_events", "app_import_items",
	"app_import_sessions", "app_schedule_history", "app_spend_cap_events", "audit_chain_links",
	"audit_log", "backup_history", "build_runs", "database_query_history", "deploy_attempts",
	"deploy_exit_captures", "deploy_gate_events", "deploy_trace_steps", "env_changes",
	"event_webhook_deliveries", "fixit_attempts", "forge_deployments", "gate_events",
	"gate_sessions", "idempotency_keys", "janitor_runs", "log_archive_runs", "model_usage_hourly",
	"node_security_events", "node_status_events", "pipeline_jobs", "pipeline_logs",
	"pipeline_runs", "pipeline_steps", "pipeline_trigger_log", "pipeline_webhook_deliveries",
	"placement_decisions", "probe_attempts", "restore_history", "scheduled_task_runs",
	"search_usage", "secret_scan_runs", "service_hook_runs", "session_activity", "sleep_events",
	"status_component_daily", "team_events", "theauth_audit_events", "theauth_sessions",
	"theauth_signal_queue", "upgrade_history", "usage_cost_daily", "usage_cost_nodes",
	"webhook_deliveries",
}

// rebuildBaseline freezes the pre-2300 migrations that already rebuild an
// unbounded table. It must only shrink; new files never belong here.
var rebuildBaseline = map[string]bool{}

var (
	createTableRe = regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)
	dropTableRe   = regexp.MustCompile(`(?i)DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?([a-z0-9_]+)`)
)

// rebuiltTables returns the tables a migration drops while creating a sibling
// whose name contains it (the CREATE x_new, copy, DROP x, RENAME pattern).
func rebuiltTables(sql string) []string {
	var created []string
	for _, m := range createTableRe.FindAllStringSubmatch(sql, -1) {
		created = append(created, strings.ToLower(m[1]))
	}
	var out []string
	for _, m := range dropTableRe.FindAllStringSubmatch(sql, -1) {
		dropped := strings.ToLower(m[1])
		for _, c := range created {
			if c != dropped && strings.Contains(c, dropped) {
				out = append(out, dropped)
				break
			}
		}
	}
	return out
}

func largeTableRebuilds(sql string) []string {
	large := map[string]bool{}
	for _, t := range unboundedTables {
		large[t] = true
	}
	var out []string
	for _, t := range rebuiltTables(sql) {
		if large[t] {
			out = append(out, t)
		}
	}
	return out
}

func hasReviewedMarkerWithReason(sql string) bool {
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, largeTableRebuildMarker); ok {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rest), ":")) != ""
		}
	}
	return false
}

func TestMigrationLint_LargeTableRebuilds(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	fileOf := map[int]string{}
	for _, e := range entries {
		v, _, _ := parseMigrationFilename(e.Name())
		fileOf[v] = e.Name()
	}
	for _, m := range migs {
		file := fileOf[m.version]
		hits := largeTableRebuilds(m.sql)
		if len(hits) == 0 || rebuildBaseline[file] {
			continue
		}
		if !hasReviewedMarkerWithReason(m.sql) {
			t.Errorf("%s rebuilds unbounded table(s) %v: add %q plus a one-line justification, or alter in place", file, hits, largeTableRebuildMarker)
		}
	}
	for f := range rebuildBaseline {
		found := false
		for _, file := range fileOf {
			if file == f {
				found = true
			}
		}
		if !found {
			t.Errorf("rebuildBaseline lists %s which no longer exists", f)
		}
	}
}

func TestMigrationLint_RebuildDetection(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"classic rebuild", "CREATE TABLE audit_log_new (a TEXT);\nINSERT INTO audit_log_new SELECT a FROM audit_log;\nDROP TABLE audit_log;\nALTER TABLE audit_log_new RENAME TO audit_log;", []string{"audit_log"}},
		{"if exists drop", "create table if not exists deploy_attempts_tmp (a);\ndrop table if exists deploy_attempts;", []string{"deploy_attempts"}},
		{"plain add column", "ALTER TABLE audit_log ADD COLUMN x TEXT;", nil},
		{"small table rebuild", "CREATE TABLE users_new (a);\nDROP TABLE users;", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := largeTableRebuilds(tc.sql)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if hasReviewedMarkerWithReason(largeTableRebuildMarker + "\n") {
		t.Error("marker without a justification must not pass")
	}
	if !hasReviewedMarkerWithReason(largeTableRebuildMarker + ": table is capped at 1000 rows\n") {
		t.Error("marker with a justification must pass")
	}
}

func TestMigrationLint_VersionsStrictlyIncreasing(t *testing.T) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	if !sort.StringsAreSorted(files) {
		t.Fatal("ReadDir order is not filename order")
	}
	prev := 0
	for _, f := range files {
		v, _, err := parseMigrationFilename(f)
		if err != nil {
			t.Fatal(err)
		}
		if v <= prev {
			t.Errorf("%s: version %d is not greater than the previous %d (duplicate or out of filename order; pad to 4 digits)", f, v, prev)
		}
		prev = v
	}
}

func TestMigrationLint_HeaderComment(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if m.version < headerRequiredFrom {
			continue
		}
		first := strings.TrimSpace(strings.SplitN(m.sql, "\n", 2)[0])
		if !strings.HasPrefix(first, "--") || strings.Trim(first, "- ") == "" {
			t.Errorf("migration %04d_%s must start with a header comment line saying why it exists", m.version, m.name)
		}
	}
}
