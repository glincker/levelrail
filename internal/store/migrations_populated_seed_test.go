package store

import (
	"context"
	"testing"
)

// populatedPostTagSeed fills only the tables that migrations above 3199 alter or
// rebuild, using the v0.9.2 column set. It runs after the base seed (service web).
var populatedPostTagSeed = []string{
	`INSERT INTO service_domains (domain, service_name) VALUES ('app.example.test', 'web')`,
	`INSERT INTO domain_redirect (domain, target_url, status_code) VALUES ('app.example.test', 'https://new.example.test', 308)`,
	`INSERT INTO domain_auth_gate (domain, enabled, auth_url, updated_at) VALUES ('app.example.test', 1, 'https://auth.example.test/check', '2026-10-01T00:00:00Z')`,
	`INSERT INTO domain_maintenance (domain) VALUES ('app.example.test')`,
	`INSERT INTO static_sites (name, root_dir, updated_at) VALUES ('docs', '/srv/docs', '2026-10-01T00:00:00Z')`,
	`INSERT INTO static_site_domains (domain, static_site_name) VALUES ('docs.example.test', 'docs')`,
	`UPDATE upgrade_policy SET mode = 'patch', min_release_age_days = 5, window_cron = '0 3 * * 0' WHERE id = 1`,
	`INSERT INTO node_drains (id, node_id, status, started_at, updated_at) VALUES ('d1', 'n1', 'succeeded', '2026-10-01T00:00:00Z', '2026-10-01T00:05:00Z')`,
	`INSERT INTO node_drains (id, node_id, status, blocked, started_at, updated_at) VALUES ('d2', 'n2', 'running', '["db"]', '2026-10-02T00:00:00Z', '2026-10-02T00:00:00Z')`,
	`INSERT INTO scheduled_tasks (id, service_name, command, schedule, created_at, updated_at) VALUES ('t1', 'web', 'true', '* * * * *', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z')`,
	`INSERT INTO scheduled_task_http (task_id, path) VALUES ('t1', '/cron')`,
}

// assertPopulatedPostTag checks counts, kept values and default backfills after
// every newer migration has run over the seed.
func assertPopulatedPostTag(ctx context.Context, t *testing.T, db *DB) {
	t.Helper()
	one := func(q string, dest ...any) {
		t.Helper()
		if err := db.QueryRowContext(ctx, q).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	count := func(q string) int {
		t.Helper()
		var n int
		one(q, &n)
		return n
	}

	for tbl, want := range map[string]int{"service_domains": 1, "domain_redirect": 1, "domain_auth_gate": 1, "domain_maintenance": 1, "static_site_domains": 1, "node_drains": 2, "scheduled_task_http": 1, "upgrade_policy": 1} {
		if n := count(`SELECT COUNT(*) FROM ` + tbl); n != want {
			t.Errorf("%s rows = %d, want %d", tbl, n, want)
		}
	}

	var target string
	var code int
	one(`SELECT target_url, status_code FROM domain_redirect WHERE domain = 'app.example.test'`, &target, &code)
	if target != "https://new.example.test" || code != 308 {
		t.Errorf("redirect = %q %d", target, code)
	}
	var gateURL, provider string
	var enabled int
	one(`SELECT auth_url, enabled, provider FROM domain_auth_gate WHERE domain = 'app.example.test'`, &gateURL, &enabled, &provider)
	if gateURL != "https://auth.example.test/check" || enabled != 1 || provider != "external" {
		t.Errorf("auth gate = %q %d %q", gateURL, enabled, provider)
	}

	var mode, cron, paused, dismissed string
	var age, window int
	one(`SELECT mode, window_cron, min_release_age_days, paused_until, notice_dismissed_at, health_window_seconds FROM upgrade_policy WHERE id = 1`, &mode, &cron, &age, &paused, &dismissed, &window)
	if mode != "patch" || cron != "0 3 * * 0" || age != 5 || paused != "" || dismissed != "" || window != 0 {
		t.Errorf("upgrade_policy = %q %q %d %q %q %d", mode, cron, age, paused, dismissed, window)
	}

	var blocked, status string
	one(`SELECT status, blocked FROM node_drains WHERE id = 'd2'`, &status, &blocked)
	if status != "running" || blocked != `["db"]` {
		t.Errorf("running drain = %q %q", status, blocked)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO node_drains (id, node_id, status, started_at, updated_at) VALUES ('d3', 'n3', 'partial', 'x', 'x')`); err != nil {
		t.Errorf("partial status rejected after rebuild: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO node_drains (id, node_id, status, started_at, updated_at) VALUES ('d4', 'n2', 'running', 'x', 'x')`); err == nil {
		t.Error("second running drain for one node accepted, active index lost")
	}

	var headers string
	one(`SELECT headers FROM scheduled_task_http WHERE task_id = 't1'`, &headers)
	if headers != "[]" {
		t.Errorf("task headers backfill = %q", headers)
	}
	var nc, nic, ro string
	one(`SELECT next_cache, next_image_cache, runtime_options FROM desired_services WHERE name = 'web'`, &nc, &nic, &ro)
	if nc != "" || nic != "" || ro != "" {
		t.Errorf("desired_services backfill = %q %q %q", nc, nic, ro)
	}
	if n := count(`SELECT COUNT(*) FROM bootstrap_script_limits WHERE id = 1 AND max_bytes IS NULL AND max_count IS NULL`); n != 1 {
		t.Errorf("bootstrap_script_limits default row count = %d", n)
	}

	// Ownership triggers rebuilt by 3213 and the new limits table.
	if _, err := db.ExecContext(ctx, `INSERT INTO domain_limits (domain, max_body_bytes, updated_at) VALUES ('app.example.test', 1024, 'x')`); err != nil {
		t.Errorf("limits for owned domain: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO domain_limits (domain, updated_at) VALUES ('ghost.example.test', 'x')`); err == nil {
		t.Error("limits for unowned domain accepted")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO domain_redirect (domain, target_url) VALUES ('docs.example.test', 'https://x.example.test')`); err != nil {
		t.Fatalf("redirect on static domain: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM static_site_domains WHERE domain = 'docs.example.test'`); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM domain_redirect WHERE domain = 'docs.example.test'`); n != 0 {
		t.Errorf("static domain delete left %d redirect rows", n)
	}
	if n := count(`SELECT COUNT(*) FROM domain_redirect WHERE domain = 'app.example.test'`); n != 1 {
		t.Errorf("unrelated redirect lost, rows = %d", n)
	}
}
