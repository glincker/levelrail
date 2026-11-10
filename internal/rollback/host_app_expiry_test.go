package rollback

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/GLINCKER/levelrail/internal/store" // registers the sqlite driver
)

func TestSwapInBackup_KeepsStoppedAppExpiries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	live, backup := filepath.Join(dir, "live.db"), filepath.Join(dir, "backup.db")
	for _, p := range []string{live, backup} {
		db, err := sql.Open("sqlite", "file:"+p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `CREATE TABLE app_expiry (app_name TEXT PRIMARY KEY, stopped_at TEXT NOT NULL DEFAULT '', kept_reason TEXT NOT NULL DEFAULT '', claimed_at TEXT NOT NULL DEFAULT '')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO app_expiry (app_name, stopped_at) VALUES ('a', '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
	if _, err := SwapInBackup(live, backup, time.Now()); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+live)
	defer func() { _ = db.Close() }()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT kept_reason FROM app_expiry WHERE app_name = 'a'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "restored from backup" {
		t.Fatalf("kept_reason = %q after a rollback swap", got)
	}
}
