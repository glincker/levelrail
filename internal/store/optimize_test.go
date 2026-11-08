package store

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

func TestOptimizeLoop_RecordsRunAndStats(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.DB.ExecContext(ctx, "CREATE TABLE opt_t (a INTEGER, b INTEGER); CREATE INDEX opt_t_a ON opt_t(a)"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		if _, err := db.DB.ExecContext(ctx, "INSERT INTO opt_t VALUES (?, ?)", i%5, i); err != nil {
			t.Fatal(err)
		}
	}
	// PRAGMA optimize only analyzes tables the planner used, so run a query first.
	rows, err := db.DB.QueryContext(ctx, "SELECT b FROM opt_t WHERE a = 2")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
	}
	_ = rows.Close()

	lctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go db.RunOptimize(lctx, func() time.Duration { return 20 * time.Millisecond })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, l := range footprint.Loops() {
			if l.Name == "sqlite optimize" && l.Runs >= 1 {
				var n int
				if err := db.DB.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_stat1 WHERE tbl = 'opt_t'").Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n == 0 {
					t.Fatal("no sqlite_stat1 row for opt_t after optimize")
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("optimize loop never ran")
}
