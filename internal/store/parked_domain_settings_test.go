package store

import (
	"context"
	"slices"
	"testing"
)

var parkedSettingSeeds = map[string]string{
	"domain_waf":               `INSERT INTO domain_waf (domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst) VALUES ('a.example.com', 1, 'block', 7, 9)`,
	"domain_tls_cert":          `INSERT INTO domain_tls_cert (domain, uploaded_at, expires_at) VALUES ('a.example.com', '2026-01-01T00:00:00Z', '2027-01-01T00:00:00Z')`,
	"domain_basic_auth":        `INSERT INTO domain_basic_auth (domain, username) VALUES ('a.example.com', 'alice')`,
	"domain_auth_gate":         `INSERT INTO domain_auth_gate (domain, enabled, auth_url, check_path, copy_headers, timeout_seconds, updated_at) VALUES ('a.example.com', 1, 'https://auth.example.com', '/c', '["X-User"]', 4, '2026-01-01T00:00:00Z')`,
	"gate_access_rules":        `INSERT INTO gate_access_rules (domain, kind, subject, created_at) VALUES ('a.example.com', 'role', 'admin', '2026-01-01T00:00:00Z')`,
	"domain_maintenance":       `INSERT INTO domain_maintenance (domain) VALUES ('a.example.com')`,
	"domain_redirect":          `INSERT INTO domain_redirect (domain, target_url, status_code) VALUES ('a.example.com', 'https://new.example.com', 302)`,
	"domain_error_page":        `INSERT INTO domain_error_page (domain, status_code, body) VALUES ('a.example.com', 503, 'down')`,
	"domain_search_visibility": `INSERT INTO domain_search_visibility (domain) VALUES ('a.example.com')`,
	"domain_traffic_policies":  `INSERT INTO domain_traffic_policies (domain, kind, spec, updated_at) VALUES ('a.example.com', 'headers', '{}', '2026-01-01T00:00:00Z')`,
	"domain_limits":            `INSERT INTO domain_limits (domain, max_body_bytes, read_timeout_seconds, write_timeout_seconds, updated_at) VALUES ('a.example.com', 1048576, 30, 30, '2026-01-01T00:00:00Z')`,
	"domain_tuning":            `INSERT INTO domain_tuning (domain, preset, websocket, stream_timeout_seconds, updated_at) VALUES ('a.example.com', 'websocket', 1, 3600, '2026-01-01T00:00:00Z')`,
}

func countSettingRows(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table+` WHERE domain = 'a.example.com'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedParkedSettings(t *testing.T) (*DB, context.Context) {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()
	for _, svc := range []DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range parkedSettingTables {
		if _, err := db.ExecContext(ctx, parkedSettingSeeds[table]); err != nil {
			t.Fatalf("seed %s: %v", table, err)
		}
	}
	return db, ctx
}

func TestParkedDomain_AttachRestoresEachSetting(t *testing.T) {
	db, ctx := seedParkedSettings(t)
	if err := db.ParkServiceDomain(ctx, "web", "a.example.com", "admin", ""); err != nil {
		t.Fatal(err)
	}
	for table := range parkedSettingSeeds {
		if n := countSettingRows(t, db, table); n != 0 {
			t.Fatalf("%s kept %d rows while parked, want 0 (live tables must not route a parked domain)", table, n)
		}
	}
	p, err := db.GetParkedDomain(ctx, "a.example.com")
	if err != nil || !p.HasSettings {
		t.Fatalf("parked = %+v, %v; want HasSettings", p, err)
	}
	restored, err := db.AttachParkedDomainWith(ctx, "a.example.com", "api", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != len(parkedSettingSeeds) {
		t.Fatalf("restored = %v, want %d kinds", restored, len(parkedSettingSeeds))
	}
	for table := range parkedSettingSeeds {
		if n := countSettingRows(t, db, table); n != 1 {
			t.Fatalf("%s has %d rows after attach, want 1", table, n)
		}
	}
	var rps, burst int
	var mode string
	if err := db.QueryRowContext(ctx, `SELECT waf_mode, rate_limit_rps, rate_limit_burst FROM domain_waf WHERE domain = 'a.example.com'`).Scan(&mode, &rps, &burst); err != nil {
		t.Fatal(err)
	}
	if mode != "block" || rps != 7 || burst != 9 {
		t.Fatalf("waf = %s %d %d, want block 7 9", mode, rps, burst)
	}
}

func TestParkedDomain_AttachDiscardDropsSettings(t *testing.T) {
	db, ctx := seedParkedSettings(t)
	if err := db.ParkServiceDomain(ctx, "web", "a.example.com", "admin", ""); err != nil {
		t.Fatal(err)
	}
	restored, err := db.AttachParkedDomainWith(ctx, "a.example.com", "api", false)
	if err != nil || len(restored) != 0 {
		t.Fatalf("restored = %v, %v; want none", restored, err)
	}
	for table := range parkedSettingSeeds {
		if n := countSettingRows(t, db, table); n != 0 {
			t.Fatalf("%s has %d rows after discard, want 0", table, n)
		}
	}
	svc, _ := db.GetDesiredService(ctx, "api")
	if !slices.Contains(svc.Domains, "a.example.com") {
		t.Fatalf("api domains = %v", svc.Domains)
	}
}

func TestParkedDomain_AttachWithoutSnapshot(t *testing.T) {
	db, ctx := seedParkedSettings(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO parked_domains (domain, parked_from, parked_at) VALUES ('old.example.com', 'web', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	restored, err := db.AttachParkedDomainWith(ctx, "old.example.com", "api", true)
	if err != nil || len(restored) != 0 {
		t.Fatalf("restored = %v, %v; want none", restored, err)
	}
	svc, _ := db.GetDesiredService(ctx, "api")
	if !slices.Contains(svc.Domains, "old.example.com") {
		t.Fatalf("api domains = %v", svc.Domains)
	}
}
