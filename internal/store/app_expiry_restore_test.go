package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestKeepRestoredAppExpiries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name, stoppedAt, kept, want string
	}{
		{"stopped row is kept", "2026-01-01T00:00:00Z", "", "restored from backup"},
		{"scheduled row is untouched", "", "", ""},
		{"kept row keeps its reason", "2026-01-01T00:00:00Z", "app has bind mounts", "app has bind mounts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "cp.db")
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TABLE app_expiry (app_name TEXT PRIMARY KEY, stopped_at TEXT NOT NULL DEFAULT '', kept_reason TEXT NOT NULL DEFAULT '', claimed_at TEXT NOT NULL DEFAULT '')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO app_expiry (app_name, stopped_at, kept_reason, claimed_at) VALUES ('a', ?, ?, 'x')`, tc.stoppedAt, tc.kept); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			if err := KeepRestoredAppExpiries(ctx, path); err != nil {
				t.Fatal(err)
			}
			db, _ = sql.Open("sqlite", "file:"+path)
			defer func() { _ = db.Close() }()
			var got string
			if err := db.QueryRowContext(ctx, `SELECT kept_reason FROM app_expiry WHERE app_name = 'a'`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("kept_reason = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("database without the table is fine", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "old.db")
		db, _ := sql.Open("sqlite", "file:"+path)
		if _, err := db.ExecContext(ctx, `CREATE TABLE other (id INTEGER)`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		if err := KeepRestoredAppExpiries(ctx, path); err != nil {
			t.Fatal(err)
		}
	})
}
