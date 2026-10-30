package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// legacyBriefStatuses is the per-row SQL the one-pass derivation replaced.
func legacyBriefStatuses(t testing.TB, db *DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT `+deploymentStatusSQL+`, `+isLiveSQL+`, d.id `+deploymentFromSQL+` ORDER BY d.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var st, id string
		var live bool
		if err := rows.Scan(&st, &live, &id); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s|%s|%v", id, st, live))
	}
	return out
}

func TestDeploymentBriefsMatchPerRowSQL(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	base := time.Now().UTC().Add(-48 * time.Hour)
	for _, app := range []string{"a", "b"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: app, Image: "x:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	type row struct{ svc, image, status, source, reason, errText string }
	rows := []row{
		{"a", "v1", "succeeded", "webhook", "", ""},
		{"a", "v2", "succeeded", "webhook", "", ""},
		{"a", "v1", "succeeded", "manual", "RollbackTo:dep_x", ""},
		{"a", "v3", "failed", "webhook", "", "context canceled"},
		{"a", "v4", "failed", "webhook", "", "boom"},
		{"a", "v5", "running", "webhook", "", ""},
		{"b", "v1", "succeeded", "webhook", "", ""},
		{"b", "v2", "succeeded", "auto_rollback", "", ""},
		{"b", "v2", "succeeded", "webhook", "", ""},
		{"b", "v6", "superseded", "webhook", "", ""},
	}
	for i, r := range rows {
		st := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `INSERT INTO deploy_attempts (id, service_name, image, status, started_at, finished_at, source, reason, error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, fmt.Sprintf("d%02d", i), r.svc, r.image, r.status, st, st, r.source, r.reason, r.errText); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]int{}
	for _, l := range legacyBriefStatuses(t, db) {
		var id, st, live string
		if _, err := fmt.Sscanf(l, "%3s|%s", &id, &st); err != nil {
			t.Fatal(err)
		}
		_ = live
		want[l[4:]]++
	}
	briefs, err := db.ListDeploymentBriefs(ctx, base.Add(-time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, b := range briefs {
		got[fmt.Sprintf("%s|%v", b.Status, b.IsLive)]++
	}
	if len(briefs) != len(rows) {
		t.Fatalf("briefs = %d, want %d", len(briefs), len(rows))
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("status/live %s: got %d want %d (got=%v want=%v)", k, got[k], n, got, want)
		}
	}
}

func BenchmarkHotPathLegacyBriefs(b *testing.B) {
	db := seedHotPaths(b)
	since := time.Now().Add(-14 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := db.QueryContext(context.Background(), `SELECT `+deploymentStatusSQL+`, d.rollout_state, `+isLiveSQL+`, d.started_at, d.finished_at `+deploymentFromSQL+` WHERE d.started_at >= ?`, since)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		_ = rows.Close()
	}
}
